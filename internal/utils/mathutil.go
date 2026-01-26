package utils

// ClampInt clamps v into the inclusive [min, max] range.
// If min > max, the bounds are swapped.
func ClampInt(v, min, max int) int {
	if min > max {
		min, max = max, min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// MinInt returns the smallest value from vals. Returns 0 if empty.
func MinInt(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	min := vals[0]
	for i := 1; i < len(vals); i++ {
		if vals[i] < min {
			min = vals[i]
		}
	}
	return min
}

// MaxInt returns the largest value from vals. Returns 0 if empty.
func MaxInt(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	max := vals[0]
	for i := 1; i < len(vals); i++ {
		if vals[i] > max {
			max = vals[i]
		}
	}
	return max
}

// SumInts returns the sum of all values in vals.
func SumInts(vals ...int) int {
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return sum
}
