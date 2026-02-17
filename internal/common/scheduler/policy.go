package scheduler

// FixedPermits returns a policy that reserves a fixed number of permits per chunk.
func FixedPermits(n int) ReservePolicy {
	if n < 0 {
		n = 0
	}
	return func(_ []int) int { return n }
}

// WeightedPermits reserves a number of permits equal to the sum of weights
// of nodes in the chunk.
func WeightedPermits(weightOf WeightFn) ReservePolicy {
	if weightOf == nil {
		return func(_ []int) int { return 0 }
	}
	return func(chunk []int) int {
		sum := 0
		for _, u := range chunk {
			sum += weightOf(u)
		}
		return sum
	}
}
