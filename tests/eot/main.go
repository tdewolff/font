//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/tdewolff/font"

// Fuzz is a fuzz test.
func Fuzz(data []byte) int {
	_, _ = font.ParseEOT(data)
	return 1
}
