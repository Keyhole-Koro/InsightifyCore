package utils

// Filter returns a new slice with items that satisfy keep.
// If keep is nil, it returns a shallow copy of src.
func Filter[T any](src []T, keep func(T) bool) []T {
	if len(src) == 0 {
		return nil
	}
	if keep == nil {
		return append([]T(nil), src...)
	}
	out := make([]T, 0, len(src))
	for _, v := range src {
		if keep(v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
