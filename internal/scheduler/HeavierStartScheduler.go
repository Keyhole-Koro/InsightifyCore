package scheduler

import (
	"context"
	"errors"
	"math/big"
	"math/bits"
	"sort"

	"insightify/internal/llm"
)

//
// Public types
//

// WeightFn returns the weight of a node by its integer ID.
// The scheduler does not cache results; if weight lookup is expensive,
// wrap your data in a map or closure that does O(1) access.
type WeightFn func(nodeID int) int

// ChunkRunner executes a chunk (batch) of node IDs and returns a channel
// that is closed when the chunk has finished. The scheduler listens for the
// channel close (or context cancellation) to proceed.
type ChunkRunner func(ctx context.Context, chunk []int) (<-chan struct{}, error)

//
// ScheduleHeavierStart
//

// ScheduleHeavierStart is an event-driven scheduler that repeatedly packs ready nodes
// into chunks under a capacity constraint, prioritizing by descendant count (higher first),
// then by weight (lower first), then by node ID (lower first).
//
// Parameters:
//   - adj: DAG adjacency list where edge u->v means "u must finish before v".
//   - weightOf: function to obtain a node's weight.
//   - targets: set of nodes we care about; the scheduler stops once all targets complete.
//   - capPerChunk: maximum total weight per launched chunk.
//   - nParallel: maximum number of chunks in flight concurrently (<=0 treated as 1).
//   - run: user-supplied executor; must return a channel that closes on completion.
//
// Returns:
//   - completion times (seconds since scheduler start) for nodes that actually ran.
//
// Notes:
//   - The function assumes the graph is a DAG; it errors if a cycle is detected.
//   - If any ready node's weight exceeds capPerChunk, it returns an error.

type Params struct {
	Adj         [][]int
	WeightOf    WeightFn
	Targets     map[int]struct{}
	CapPerChunk int
	NParallel   int
	Run         ChunkRunner

	// Optional reservation integration: reserve permits before launching each chunk.
	Broker      llm.PermitBroker
	ReserveWith ReservePolicy
}

// ReservePolicy decides how many permits to reserve for a chunk.
// Returning <= 0 disables reservation for that chunk.
type ReservePolicy func(chunk []int) int

// ScheduleHeavierStart runs the event-driven scheduler using Params.
// Returns node completion times in seconds since start.
func ScheduleHeavierStart(ctx context.Context, p Params) error {
	// ---- validate & defaults ----
	if p.Run == nil {
		return errors.New("Run callback is nil")
	}
	if p.WeightOf == nil {
		return errors.New("WeightOf is nil")
	}
	if p.Adj == nil {
		return errors.New("Adj is nil")
	}
	if p.CapPerChunk <= 0 {
		return errors.New("CapPerChunk must be > 0")
	}
	nParallel := p.NParallel
	if nParallel <= 0 {
		nParallel = 1
	}

	adj := p.Adj
	weightOf := p.WeightOf
	targets := p.Targets
	capPerChunk := p.CapPerChunk
	run := p.Run

	// ---- precompute graph facts ----
	n := len(adj)
	need := computeNeededNodes(adj, targets)
	indeg := computeIndegrees(adj)

	desc, err := descendantCounts(adj)
	if err != nil {
		return err
	}

	// ---- scheduler state ----
	ready := make(IntSet, n)
	for u := 0; u < n; u++ {
		if indeg[u] == 0 {
			if _, ok := need[u]; ok {
				ready.Add(u)
			}
		}
	}
	completed := make(IntSet, n)

	type completion struct{ chunk []int }
	completionCh := make(chan completion, n)
	inflight := 0

	// Launch as many chunks as we can under nParallel.
	tryLaunch := func() error {
		for inflight < nParallel {
			// candidates = ready \ completed
			cands := make([]int, 0, len(ready))
			for u := range ready {
				if !completed.Has(u) {
					cands = append(cands, u)
				}
			}
			if len(cands) == 0 {
				break
			}

			chunk := buildChunkDesc(cands, weightOf, capPerChunk, desc, adj, indeg, need, completed)
			if len(chunk) == 0 {
				// if any candidate exceeds capacity, that is an error
				var heavy []int
				for _, u := range cands {
					if w := weightOf(u); w > capPerChunk {
						heavy = append(heavy, u)
					}
				}
				if len(heavy) > 0 {
					return errorNodeExceedsCapacity(heavy)
				}
				// otherwise we just cannot pack more right now
				break
			}

			for _, u := range chunk {
				ready.Remove(u)
			}

			// Optional: pre-reserve permits and embed credits into the launch context.
			ctxLaunch := ctx
			if p.Broker != nil && p.ReserveWith != nil {
				if want := p.ReserveWith(chunk); want > 0 {
					lease, err := p.Broker.Reserve(ctx, want)
					if err != nil {
						return err
					}
					ctxLaunch = lease.Context(ctxLaunch)
				}
			}

			doneCh, err := run(ctxLaunch, chunk)
			if err != nil {
				return err
			}
			chunkCopy := append([]int(nil), chunk...)
			go func(cc []int, ch <-chan struct{}) {
				select {
				case <-ctx.Done():
					// main loop will exit via ctx.Done
				case <-ch:
					completionCh <- completion{chunk: cc}
				}
			}(chunkCopy, doneCh)
			inflight++
		}
		return nil
	}

	// initial launch
	if err := tryLaunch(); err != nil {
		return err
	}

	// main loop
	for !isSubset(completed, targets) {
		if inflight == 0 {
			if err := tryLaunch(); err != nil {
				return err
			}
			if inflight == 0 {
				return errors.New("deadlock: nothing inflight and nothing to launch")
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-completionCh:
			inflight--

			// mark completed, timestamp, relax edges, promote new ready
			for _, u := range ev.chunk {
				if completed.Has(u) {
					continue
				}
				completed.Add(u)
				for _, v := range adj[u] {
					indeg[v]--
					if indeg[v] == 0 {
						if _, needV := need[v]; needV && !completed.Has(v) {
							ready.Add(v)
						}
					}
				}
			}
			if err := tryLaunch(); err != nil {
				return err
			}
		}
	}

	return nil
}

//
// Graph helpers
//

// computeIndegrees calculates indegree for each node.
func computeIndegrees(adj [][]int) []int {
	n := len(adj)
	indeg := make([]int, n)
	for u := 0; u < n; u++ {
		for _, v := range adj[u] {
			indeg[v]++
		}
	}
	return indeg
}

// buildReverseGraph returns the reverse adjacency list in O(E).
func buildReverseGraph(adj [][]int) [][]int {
	n := len(adj)
	rev := make([][]int, n)
	for u := 0; u < n; u++ {
		for _, v := range adj[u] {
			rev[v] = append(rev[v], u)
		}
	}
	return rev
}

// computeNeededNodes collects all ancestors of targets (including targets).
func computeNeededNodes(adj [][]int, targets map[int]struct{}) map[int]struct{} {
	rev := buildReverseGraph(adj)
	need := make(map[int]struct{}, len(targets))
	q := make([]int, 0, len(targets))
	for t := range targets {
		need[t] = struct{}{}
		q = append(q, t)
	}
	for i := 0; i < len(q); i++ {
		t := q[i]
		for _, p := range rev[t] {
			if _, ok := need[p]; !ok {
				need[p] = struct{}{}
				q = append(q, p)
			}
		}
	}
	return need
}

// toposortAny returns any topological order or an error if a cycle exists.
func toposortAny(adj [][]int) ([]int, error) {
	n := len(adj)
	indeg := computeIndegrees(adj)
	q := make([]int, 0)
	for u := 0; u < n; u++ {
		if indeg[u] == 0 {
			q = append(q, u)
		}
	}
	order := make([]int, 0, n)
	for i := 0; i < len(q); i++ {
		u := q[i]
		order = append(order, u)
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				q = append(q, v)
			}
		}
	}
	if len(order) != n {
		return nil, errors.New("graph is not a DAG (cycle detected)")
	}
	return order, nil
}

// descendantCounts computes, for each node v, the number of distinct descendants of v.
// Uses big.Int bitsets: O(E + N * machineWordCount).
func descendantCounts(adj [][]int) ([]int, error) {
	n := len(adj)
	topo, err := toposortAny(adj)
	if err != nil {
		return nil, err
	}
	sets := make([]*big.Int, n)
	for i := range sets {
		sets[i] = new(big.Int)
	}
	for i := n - 1; i >= 0; i-- {
		v := topo[i]
		b := new(big.Int)
		for _, u := range adj[v] {
			b.Or(b, sets[u])  // union with child set
			b.SetBit(b, u, 1) // include the child itself
		}
		sets[v] = b
	}
	out := make([]int, n)
	for v := 0; v < n; v++ {
		sum := 0
		for _, w := range sets[v].Bits() {
			sum += bits.OnesCount(uint(w))
		}
		out[v] = sum
	}
	return out, nil
}

// buildChunkDesc selects nodes under capacity using the priority:
// 1) larger descendant-count, 2) smaller weight, 3) smaller node id (stable tie-break).
// It performs a small lookahead within a chunk: when a node is admitted we tentatively
// decrease indegrees of its dependents and allow newly satisfied nodes into the chunk,
// so dependent chains can be packed together before launching the chunk.
func buildChunkDesc(
	cands []int,
	weightOf WeightFn,
	capPerChunk int,
	desc []int,
	adj [][]int,
	indeg []int,
	need map[int]struct{},
	completed IntSet,
) []int {
	if capPerChunk <= 0 {
		return nil
	}
	simIndeg := append([]int(nil), indeg...) // copy; we mutate during lookahead
	chunk := make([]int, 0, len(cands))
	total := 0

	ready := make(map[int]struct{}, len(cands))
	for _, u := range cands {
		ready[u] = struct{}{}
	}
	inChunk := make(map[int]struct{}, len(cands))

	for len(ready) > 0 {
		order := make([]int, 0, len(ready))
		for u := range ready {
			order = append(order, u)
		}
		sort.SliceStable(order, func(i, j int) bool {
			ui, uj := order[i], order[j]
			di, dj := desc[ui], desc[uj]
			if di != dj {
				return di > dj
			}
			wi, wj := weightOf(ui), weightOf(uj)
			if wi != wj {
				return wi < wj
			}
			return ui < uj
		})

		added := false
		for _, u := range order {
			w := weightOf(u)
			if total+w > capPerChunk {
				continue
			}
			delete(ready, u)
			inChunk[u] = struct{}{}
			chunk = append(chunk, u)
			total += w
			added = true

			for _, v := range adj[u] {
				simIndeg[v]--
				if simIndeg[v] == 0 {
					if completed.Has(v) {
						continue
					}
					if _, ok := need[v]; !ok {
						continue
					}
					if _, ok := inChunk[v]; ok {
						continue
					}
					if _, ok := ready[v]; ok {
						continue
					}
					ready[v] = struct{}{}
				}
			}

			break
		}
		if !added {
			break
		}
	}

	return chunk
}

//
// Small utilities
//

type IntSet map[int]struct{}

func (s IntSet) Add(x int)      { s[x] = struct{}{} }
func (s IntSet) Remove(x int)   { delete(s, x) }
func (s IntSet) Has(x int) bool { _, ok := s[x]; return ok }
func (s IntSet) Len() int       { return len(s) }

func isSubset(a IntSet, b map[int]struct{}) bool {
	for x := range b {
		if !a.Has(x) {
			return false
		}
	}
	return true
}

func errorNodeExceedsCapacity(nodes []int) error {
	sort.Ints(nodes)
	return errors.New("node(s) exceed capacity: " + intsToString(nodes))
}

// intsToString is a minimal formatter to avoid importing fmt.
func intsToString(xs []int) string {
	if len(xs) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(xs)*3)
	b = append(b, '[')
	for i, x := range xs {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, itoa(x)...)
	}
	b = append(b, ']')
	return string(b)
}

// itoa is a tiny integer-to-bytes helper to avoid fmt.
func itoa(x int) []byte {
	if x == 0 {
		return []byte{'0'}
	}
	sign := false
	if x < 0 {
		sign = true
		x = -x
	}
	var buf [20]byte
	i := len(buf)
	for x > 0 {
		i--
		buf[i] = byte('0' + x%10)
		x /= 10
	}
	if sign {
		i--
		buf[i] = '-'
	}
	return buf[i:]
}
