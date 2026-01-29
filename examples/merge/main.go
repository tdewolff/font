package main

import (
	"os"

	"github.com/tdewolff/font"
)

func main() {
	// Load two fonts
	fontBytes, err := os.ReadFile("A-Z.ttf")
	if err != nil {
		panic(err)
	}
	font2Bytes, err := os.ReadFile("0-9.ttf")
	if err != nil {
		panic(err)
	}

	sfnt, err := font.ParseSFNT(fontBytes, 0)
	if err != nil {
		panic(err)
	}
	sfnt2, err := font.ParseSFNT(font2Bytes, 0)
	if err != nil {
		panic(err)
	}

	// Add all glyphs of the second font to the first
	options := font.MergeOptions{
		RearrangeCmap: false, // don't rewrite unicode mapping to sequential order
	}
	if err := sfnt.Merge(sfnt2, options); err != nil {
		panic(err)
	}

	if err := os.WriteFile("out.ttf", sfnt.Write(), 0644); err != nil {
		panic(err)
	}
}
