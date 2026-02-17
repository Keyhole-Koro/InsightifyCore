package scan

// Stream walks the repo and streams FileVisit entries over a channel.
// If filesOnly is true, directory entries are omitted.
// It always respects Options (MaxDepth, IgnoreDirs, caching flags).
// errCh receives a single error (nil on success).
func Stream(root string, opts Options, filesOnly bool) (<-chan FileVisit, <-chan error) {
	out := make(chan FileVisit, 32)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		err := ScanWithOptions(root, opts, func(fv FileVisit) {
			if filesOnly && fv.IsDir {
				return
			}
			out <- fv
		})
		errCh <- err
		close(errCh)
	}()

	return out, errCh
}
