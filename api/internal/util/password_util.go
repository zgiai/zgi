package util

import "unicode"

// ContainsLetter checks if string contains letters
func ContainsLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// ContainsNumber checks if string contains numbers
func ContainsNumber(s string) bool {
	for _, r := range s {
		if unicode.IsNumber(r) {
			return true
		}
	}
	return false
}
