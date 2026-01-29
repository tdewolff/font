package main

import (
	"os"

	"github.com/tdewolff/font"
)

func main() {
	fontBytes, err := os.ReadFile("../../resources/DejaVuSerif.ttf")
	if err != nil {
		panic(err)
	}

	sfnt, err := font.ParseSFNT(fontBytes, 0)
	if err != nil {
		panic(err)
	}

	var glyphIDs []uint16
	for c := 'A'; c <= 'Z'; c++ {
		if glyphID := sfnt.GlyphIndex(c); glyphID != 0 {
			glyphIDs = append(glyphIDs, glyphID)
		}
	}

	// Create subset
	options := font.SubsetOptions{
		Tables: font.KeepMinTables, // keep only required tables
	}
	sfntSubset, err := sfnt.Subset(glyphIDs, options)
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile("out.ttf", sfntSubset.Write(), 0644); err != nil {
		panic(err)
	}
}
