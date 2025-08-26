package util

func DivCeil(a, b int64) int64 {
	if b == 0 {
		panic("division by zero")
	}
	return (a + b - 1) / b // rounds up
}
