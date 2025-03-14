package main

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/font"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	"golang.org/x/image/vector"
)

type Show struct {
	Index   int     `short:"i" desc:"Font index for font collections"`
	GlyphID uint16  `short:"g" name:"glyph" desc:"Glyph ID"`
	Char    string  `short:"c" desc:"Literal character"`
	Unicode string  `short:"u" desc:"Unicode codepoint"`
	Width   int     `desc:"Image width"`
	PPEM    uint16  `default:"40" desc:"Pixels per em-square"`
	Scale   int     `default:"4" desc:"Image scale"`
	Ratio   float64 `desc:"Image width/height ratio"`
	Output  string  `short:"o" desc:"Output filename"`
	Input   string  `index:"0" desc:"Input file"`
}

func (cmd *Show) Run() error {
	terminal := cmd.Output == "" || cmd.Output == "-"

	b, err := ioutil.ReadFile(cmd.Input)
	if err != nil {
		return err
	}

	sfnt, err := font.ParseSFNT(b, cmd.Index)
	if err != nil {
		return err
	}

	if cmd.Char != "" {
		rs := []rune(cmd.Char)
		if len(rs) != 1 {
			return fmt.Errorf("char must be one character")
		}
		cmd.GlyphID = sfnt.GlyphIndex(rs[0])
	} else if cmd.Unicode != "" {
		codepoint, err := strconv.ParseInt(cmd.Unicode, 16, 32)
		if err != nil {
			return fmt.Errorf("invalid unicode codepoint: %v", err)
		} else if codepoint < 0 {
			return fmt.Errorf("invalid unicode codepoint: U+%4X\n", codepoint)
		}
		cmd.GlyphID = sfnt.GlyphIndex(rune(codepoint))
		if cmd.GlyphID == 0 {
			return fmt.Errorf("glyph not found for U+%4X\n", codepoint)
		}
	}
	fmt.Println("GlyphID:", cmd.GlyphID)
	fmt.Printf("Char: %v (%v)\n", printableRune(sfnt.Cmap.ToUnicode(cmd.GlyphID)), sfnt.Cmap.ToUnicode(cmd.GlyphID))
	if name := sfnt.GlyphName(cmd.GlyphID); name != "" {
		fmt.Println("Name:", name)
	}

	if cmd.Width != 0 {
		if cmd.Width < 0 {
			return fmt.Errorf("width must be positive")
		}
		cmd.PPEM = uint16(float64(cmd.Width) * float64(sfnt.Head.UnitsPerEm) / float64(sfnt.GlyphAdvance(cmd.GlyphID)))
	}

	ascent := sfnt.Hhea.Ascender
	descent := -sfnt.Hhea.Descender
	width := int(float64(cmd.PPEM)*float64(sfnt.GlyphAdvance(cmd.GlyphID))/float64(sfnt.Head.UnitsPerEm) + 0.5)
	height := int(float64(cmd.PPEM)*float64(ascent+descent)/float64(sfnt.Head.UnitsPerEm) + 0.5)
	//baseline := int(float64(ppem)*float64(ascent)/float64(sfnt.Head.UnitsPerEm) + 0.5)
	xpadding := int(float64(width)*0.2 + 0.5)
	ypadding := xpadding
	if terminal {
		ypadding = 0
	}

	if 2048 < width {
		return fmt.Errorf("width cannot exceed 2048")
	}

	f := float64(cmd.PPEM) / float64(sfnt.Head.UnitsPerEm)

	p := &canvas.Path{}
	err = sfnt.GlyphPath(p, cmd.GlyphID, cmd.PPEM, 0.0, float64(descent), 1.0, font.NoHinting)
	if err != nil {
		return err
	}

	rect := image.Rect(0, 0, width+2*xpadding, height+2*ypadding)
	glyphRect := image.Rect(xpadding, ypadding, width+xpadding, height+ypadding)

	img := image.NewRGBA(rect)
	draw.Draw(img, rect, image.NewUniform(canvas.White), image.ZP, draw.Over)

	ras := vector.NewRasterizer(width, height)
	p.ToVectorRasterizer(ras, canvas.DPMM(f))
	ras.Draw(img, glyphRect, image.NewUniform(canvas.Black), image.ZP)

	if cmd.Ratio == 0.0 {
		if terminal {
			cmd.Ratio = 2.0
		} else {
			cmd.Ratio = 1.0
		}
	}

	if cmd.Ratio != 1.0 {
		origImg := img
		origRect := rect
		rect := image.Rect(0, 0, int(float64(origRect.Max.X)*cmd.Ratio+0.5), origRect.Max.Y)
		img = image.NewRGBA(rect)
		draw.ApproxBiLinear.Scale(img, rect, origImg, origRect, draw.Over, nil)
	}

	if terminal {
		if 80 < width {
			return fmt.Errorf("width cannot exceed 80 for terminal output")
		}
		printASCII(img)
		return nil
	}

	if cmd.Scale != 1 {
		origImg := img
		origRect := rect
		rect := image.Rect(0, 0, (origRect.Max.X)*cmd.Scale, (origRect.Max.Y)*cmd.Scale)
		img = image.NewRGBA(rect)
		draw.NearestNeighbor.Scale(img, rect, origImg, origRect, draw.Over, nil)
	}

	ext := filepath.Ext(cmd.Output)
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".tiff" {
		return fmt.Errorf("output extension must be PNG, JPG, GIF, or TIFF")
	}

	w, err := os.Create(cmd.Output)
	if err != nil {
		return err
	}

	switch ext {
	case ".png":
		err = png.Encode(w, img)
	case ".jpg", ".jpeg":
		err = jpeg.Encode(w, img, nil)
	case ".gif":
		err = gif.Encode(w, img, nil)
	case ".tiff":
		err = tiff.Encode(w, img, nil)
	}

	if err != nil {
		return err
	}
	return nil
}
