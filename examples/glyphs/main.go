package main

import (
	"fmt"
	"os"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers"
	"github.com/tdewolff/font"
)

const mmPerPt = 25.4 / 72.0

func main() {
	// Can be any of TTF, OTF, TTC, WOFF, WOFF2, or EOT
	fontBytes, err := os.ReadFile("../../resources/DejaVuSerif.ttf")
	if err != nil {
		panic(err)
	}

	sfnt, err := font.ParseSFNT(fontBytes, 0) // 0 is the first index for TTC fonts
	if err != nil {
		panic(err)
	}

	dpmm := 30.0     // pixels per mm, rasterisation resolution
	fontSize := 12.0 // font size in points

	scale := fontSize * mmPerPt / float64(sfnt.UnitsPerEm()) // mm per units-per-em
	ppem := uint16(dpmm*fontSize*mmPerPt + 0.5)              // size of em in pixels
	ascender, descender, _ := sfnt.VerticalMetrics()

	var x, y int16
	p := &canvas.Path{}

	indexA := sfnt.GlyphIndex('A') // can be 0 if not present
	indexV := sfnt.GlyphIndex('V') // can be 0 if not present
	if err := sfnt.GlyphPath(p, indexA, ppem, scale*float64(x), scale*float64(y), scale, font.NoHinting); err != nil {
		panic(err)
	}
	x += int16(sfnt.GlyphAdvance(indexA))
	x += sfnt.Kerning(indexA, indexV)

	if err := sfnt.GlyphPath(p, indexV, ppem, scale*float64(x), scale*float64(y), scale, font.NoHinting); err != nil {
		panic(err)
	}
	x += int16(sfnt.GlyphAdvance(indexV))

	width := scale * float64(x)
	height := scale * float64(ascender+descender)
	fmt.Println(scale, x, ascender+descender)

	c := canvas.New(width, height)
	ctx := canvas.NewContext(c)
	ctx.DrawPath(0.0, 0.0, p)
	if err := renderers.Write("out.png", c, canvas.Resolution(dpmm)); err != nil {
		panic(err)
	}
}
