package main

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tdewolff/argp"
	"github.com/tdewolff/font"
	"github.com/tdewolff/prompt"
)

func main() {
	glyphs := []string{}
	chars := []string{}
	names := []string{}
	unicodes := []string{}
	unicodeRanges := []string{}
	index := 0
	var input, output string

	cmd := argp.New("Subset TTF/OTF/WOFF/WOFF2/EOT/TTC/OTC font file")
	cmd.AddOpt(argp.Append{&glyphs}, "g", "glyph", "List of glyph IDs to keep, eg. 1-100.")
	cmd.AddOpt(argp.Append{&chars}, "c", "char", "List of literal characters to keep, eg. a-z.")
	cmd.AddOpt(argp.Append{&names}, "n", "name", "List of glyph names to keep, eg. space.")
	cmd.AddOpt(argp.Append{&unicodes}, "u", "unicode", "List of unicode IDs to keep, eg. f0fc-f0ff.")
	cmd.AddOpt(argp.Append{&unicodeRanges}, "r", "range", "List of unicode categories or scripts to keep, eg. L (for Letters) or Latin.")
	cmd.AddOpt(&index, "", "index", "Index into font collection (used with TTC or OTC).")
	cmd.AddOpt(&output, "o", "output", "Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported).")
	cmd.AddVal(&input, "input", "Input font file.")
	cmd.Parse()

	if output == "" {
		output = input
	}

	// read from file and parse font
	var err error
	var r *os.File
	if input == "" || input == "-" {
		r = os.Stdin
	} else if r, err = os.Open(input); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		r.Close()
		fmt.Println("ERROR:", err)
		os.Exit(1)
	} else if err := r.Close(); err != nil {
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

	glyphMap := map[uint16]bool{}
	glyphMap[0] = true

	// append glyphs
	for _, glyph := range glyphs {
		if dash := strings.IndexByte(glyph, '-'); dash != -1 {
			first, err := strconv.ParseInt(glyph[:dash], 10, 16)
			if err != nil {
				fmt.Println("ERROR: invalid glyph ID:", err)
				os.Exit(1)
			}
			last, err := strconv.ParseInt(glyph[dash+1:], 10, 16)
			if err != nil {
				fmt.Println("ERROR: invalid glyph ID:", err)
				os.Exit(1)
			}
			if last < first || first < 0 || 65535 < last {
				fmt.Printf("ERROR: invalid glyph ID range: %d-%d\n", first, last)
				os.Exit(1)
			}
			for first != last+1 {
				glyphMap[uint16(first)] = true
			}
		} else {
			glyphID, err := strconv.ParseInt(glyph, 10, 16)
			if err != nil {
				fmt.Println("ERROR: invalid glyph ID:", err)
				os.Exit(1)
			}
			if glyphID < 0 || 65535 < glyphID {
				fmt.Println("ERROR: invalid glyph ID:", glyphID)
				os.Exit(1)
			}
			glyphMap[uint16(glyphID)] = true
		}
	}

	// append characters
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
						glyphMap[glyphID] = true
					}
				}
				rangeChars = false
				prev = -1
			} else {
				glyphID := sfnt.GlyphIndex(r)
				if glyphID == 0 {
					fmt.Println("WARNING: glyph not found:", string(r))
				} else {
					glyphMap[glyphID] = true
				}
				prev = r
			}
		}
		if rangeChars {
			glyphID := sfnt.GlyphIndex('-')
			if glyphID == 0 {
				fmt.Println("WARNING: glyph not found: -")
			} else {
				glyphMap[glyphID] = true
			}
		}
	}

	// append glyph names
	for _, name := range names {
		glyphID := sfnt.FindGlyphName(name)
		if glyphID == 0 {
			fmt.Println("WARNING: glyph name not found:", name)
		} else {
			glyphMap[glyphID] = true
		}
	}

	// append unicode
	for _, code := range unicodes {
		if dash := strings.IndexByte(code, '-'); dash != -1 {
			first, err := strconv.ParseInt(code[:dash], 16, 32)
			if err != nil {
				fmt.Println("ERROR: invalid unicode codepoint:", err)
				os.Exit(1)
			}
			last, err := strconv.ParseInt(code[dash+1:], 16, 32)
			if err != nil {
				fmt.Println("ERROR: invalid unicode codepoint:", err)
				os.Exit(1)
			}
			if last < first || first < 0 {
				fmt.Printf("ERROR: invalid unicode range: U+%4X-U+%4X\n", first, last)
				os.Exit(1)
			}
			for first != last+1 {
				glyphID := sfnt.GlyphIndex(rune(first))
				if glyphID == 0 {
					fmt.Printf("WARNING: glyph not found for U+%4X\n", first)
				} else {
					glyphMap[glyphID] = true
				}
			}
		} else {
			codepoint, err := strconv.ParseInt(code, 16, 32)
			if err != nil {
				fmt.Println("ERROR: invalid unicode codepoint:", err)
				os.Exit(1)
			} else if codepoint < 0 {
				fmt.Printf("ERROR: invalid unicode codepoint: U+%4X\n", codepoint)
				os.Exit(1)
			}
			glyphID := sfnt.GlyphIndex(rune(codepoint))
			if glyphID == 0 {
				fmt.Printf("WARNING: glyph not found for U+%4X\n", codepoint)
			} else {
				glyphMap[glyphID] = true
			}
		}
	}

	// append unicode ranges
	for _, unicodeRange := range unicodeRanges {
		var ok bool
		var table *unicode.RangeTable
		if table, ok = unicode.Categories[unicodeRange]; !ok {
			if table, ok = unicode.Scripts[unicodeRange]; !ok {
				fmt.Println("ERROR: invalid unicode range:", unicodeRange)
				os.Exit(1)
			}
		}
		for _, ran := range table.R16 {
			for r := ran.Lo; r <= ran.Hi; r += ran.Stride {
				glyphID := sfnt.GlyphIndex(rune(r))
				if glyphID != 0 {
					glyphMap[glyphID] = true
				}

			}
		}
		for _, ran := range table.R32 {
			for r := ran.Lo; r <= ran.Hi; r += ran.Stride {
				glyphID := sfnt.GlyphIndex(rune(r))
				if glyphID != 0 {
					glyphMap[glyphID] = true
				}
			}
		}
	}

	// convert to sorted list, prevents duplicates
	glyphIDs := make([]uint16, 0, len(glyphMap))
	for glyphID := range glyphMap {
		glyphIDs = append(glyphIDs, glyphID)
	}
	sort.Slice(glyphIDs, func(i, j int) bool { return glyphIDs[i] < glyphIDs[j] })

	// subset font
	sfnt = sfnt.Subset(glyphIDs, font.SubsetOptions{Tables: font.KeepMinTables})

	// create font program
	rLen := len(b)
	switch ext := filepath.Ext(output); ext {
	case ".ttf", ".otf", ".ttc", ".otc":
		b = sfnt.Write()
	case ".woff2":
		if b, err = sfnt.WriteWOFF2(); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("ERROR: unsupported output file extension:", ext)
		os.Exit(1)
	}
	wLen := len(b)
	ratio := 1.0
	if 0 < rLen {
		ratio = float64(wLen) / float64(rLen)
	}
	fmt.Println("Number of glyphs:", len(glyphIDs))
	fmt.Printf("File size: %6v => %6v (%.1f%%)\n", formatBytes(uint64(rLen)), formatBytes(uint64(wLen)), ratio*100.0)

	// write to file
	var w *os.File
	if output == "" || output == "-" {
		w = os.Stdout
	} else {
		if _, err := os.Stat(output); err == nil {
			if !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", output), false) {
				return
			}
		}
		if w, err = os.Create(output); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	}

	if _, err := w.Write(b); err != nil {
		w.Close()
		fmt.Println("error:", err)
		os.Exit(1)
	} else if err := w.Close(); err != nil {
		fmt.Println("error:", err)
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
