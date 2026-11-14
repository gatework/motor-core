package pkg

import (
	"math"
	"strconv"
)

// IntToString formats a signed integer in base ten.
func IntToString(e int) string {
	return strconv.Itoa(e)
}

// UIntToString formats an unsigned integer in base ten.
func UIntToString(e uint) string {
	return strconv.Itoa(int(e))
}

// Int64ToString formats a signed 64-bit integer in base ten.
func Int64ToString(e int64) string {
	return strconv.FormatInt(e, 10)
}

// Round rounds a number to the requested decimal precision.
func Round(f float64, n int) float64 {
	pow10N := math.Pow10(n)
	return math.Trunc((f+0.5/pow10N)*pow10N) / pow10N // Add half a unit before truncating to round.
}
