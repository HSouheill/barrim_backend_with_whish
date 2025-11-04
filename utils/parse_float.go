package utils

import (
	"strconv"
)

// ParseFloat converts a string to a float64, returning 0 if there's an error
func ParseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}

	return value, nil
}
