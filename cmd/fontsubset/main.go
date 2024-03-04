package main

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"

	"github.com/tdewolff/argp"
	"github.com/tdewolff/canvas/font"
	"github.com/tdewolff/prompt"
)

func main() {
	chars := []string{} //" !\"%'(),.0-9:;?@A-Za-z-"}
	names := []string{}
	glyphs := []int{}
	index := 0
	var input, output string

	cmd := argp.New("Subset TTF/OTF/TTC/WOFF/WOFF2/EOT font file")
	cmd.AddOpt(argp.Append{&chars}, "c", "char", "List of characters to keep.")
	cmd.AddOpt(argp.Append{&names}, "n", "name", "List of glyph names to keep.")
	cmd.AddOpt(argp.Append{&glyphs}, "g", "glyph", "List of glyph IDs to keep.")
	cmd.AddOpt(&index, "", "index", "Index into font collection (used for .ttc).")
	cmd.AddOpt(&output, "o", "output", "Output font file.")
	cmd.AddVal(&input, "input", "Input font file.")
	cmd.Parse()

	if input == "" {
		fmt.Println("ERROR: missing input font name")
		os.Exit(1)
	} else if output == "" {
		output = input
	}

	r, err := os.Open(input)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	b, err := ioutil.ReadAll(r)
	r.Close()
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	} else if b, err = font.ToSFNT(b); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	sfnt, err := font.ParseSFNT(b, index)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}

	fmt.Println(sfnt.Glyf.Contour(0, 0))
	fmt.Println(sfnt.Glyf.Contour(1, 0))
	fmt.Println(len(sfnt.Glyf.Get(0)))
	fmt.Println(len(sfnt.Glyf.Get(1)))

	glyphIDs := []uint16{0}
	for _, glyph := range glyphs {
		if glyph < 0 || 65535 < glyph {
			fmt.Println("WARNING: invalid glyph:", glyph)
		} else {
			glyphIDs = append(glyphIDs, uint16(glyph))
		}
	}
	for _, s := range chars {
		prev := rune(-1)
		rangeChars := false
		for _, r := range s {
			if prev != -1 && r == '-' {
				rangeChars = true
			} else if rangeChars {
				for i := prev + 1; i <= r; i++ {
					glyphID := sfnt.GlyphIndex(i)
					if glyphID == 0 {
						fmt.Println("WARNING: glyph not found:", string(i))
					} else {
						glyphIDs = append(glyphIDs, glyphID)
					}
				}
				rangeChars = false
				prev = -1
			} else {
				glyphID := sfnt.GlyphIndex(r)
				if glyphID == 0 {
					fmt.Println("WARNING: glyph not found:", string(r))
				} else {
					glyphIDs = append(glyphIDs, glyphID)
				}
				prev = r
			}
		}
		if rangeChars {
			glyphID := sfnt.GlyphIndex('-')
			if glyphID == 0 {
				fmt.Println("WARNING: glyph not found: -")
			} else {
				glyphIDs = append(glyphIDs, glyphID)
			}
		}
	}
	for _, name := range names {
		glyphID := sfnt.FindGlyphName(name)
		if glyphID == 0 {
			fmt.Println("WARNING: glyph name not found:", name)
		} else {
			glyphIDs = append(glyphIDs, glyphID)
		}
	}

	if _, err := os.Stat(output); err == nil {
		if !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", output), false) {
			return
		}
		fmt.Println()
	}

	fmt.Println("Number of glyphs:", len(glyphIDs))
	rLen := len(b)
	b, _ = sfnt.Subset(glyphIDs, font.WriteMinTables)
	wLen := len(b)

	ratio := 1.0
	if 0 < rLen {
		ratio = float64(wLen) / float64(rLen)
	}
	fmt.Printf("File size: %6v => %6v (%.1f%%)\n", formatBytes(uint64(rLen)), formatBytes(uint64(wLen)), ratio*100.0)

	w, err := os.Create(output)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	defer w.Close()

	if _, err := w.Write(b); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func formatBytes(size uint64) string {
	if size < 10 {
		return fmt.Sprintf("%d B", size)
	}

	units := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	scale := int(math.Floor((math.Log10(float64(size)) + math.Log10(2.0)) / 3.0))
	value := float64(size) / math.Pow10(scale*3.0)
	format := "%.0f %s"
	if value < 10.0 {
		format = "%.1f %s"
	}
	return fmt.Sprintf(format, value, units[scale])
}
