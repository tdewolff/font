package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tdewolff/font"
)

type Subset struct {
	Quiet         bool     `short:"q" desc:"Suppress output except for errors."`
	Force         bool     `short:"f" desc:"Force overwriting existing files."`
	Glyphs        []string `short:"g" name:"glyph" desc:"List of glyph IDs to keep, eg. 1-100."`
	Chars         []string `short:"c" name:"char" desc:"List of literal characters to keep, eg. a-z."`
	Names         []string `short:"n" name:"name" desc:"List of glyph names to keep, eg. space."`
	Unicodes      []string `short:"u" name:"unicode" desc:"List of unicode IDs to keep, eg. f0fc-f0ff."`
	UnicodeRanges []string `short:"r" name:"range" desc:"List of unicode categories or scripts to keep, eg. L (for Letters) or Latin (latin script). See https://pkg.go.dev/unicode for all supported values."`
	Index         int      `short:"i" desc:"Index into font collection (used with TTC or OTC)."`
	Type          string   `short:"t" desc:"Explicitly set output mimetype, eg. font/woff2."`
	Encoding      string   `short:"e" desc:"Output encoding, either empty of base64."`
	GlyphName     string   `desc:"New glyph name. Available variables: %i glyph ID, %n glyph name, %u glyph unicode in hexadecimal."`
	Outputs       []string `short:"o" desc:"Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported). Can output multiple file."`
	Input         string   `index:"0" desc:"Input font file."`
}

func (cmd *Subset) Run() error {
	if cmd.Quiet {
		Warning = log.New(ioutil.Discard, "", 0)
	}

	if len(cmd.Outputs) == 0 {
		cmd.Outputs = []string{cmd.Input}
	} else if cmd.Encoding != "" && cmd.Encoding != "base64" {
		return fmt.Errorf("unsupported encoding: %v", cmd.Encoding)
	}

	// read from file and parse font
	sfnt, rMimetype, rLen, err := readFont(cmd.Input, cmd.Index)
	if err != nil {
		if cmd.Input == "-" {
			return err
		}
		return fmt.Errorf("%v: %v", cmd.Input, err)
	}

	glyphMap := map[uint16]bool{}
	glyphMap[0] = true

	// append glyphs
	for _, glyph := range cmd.Glyphs {
		if dash := strings.IndexByte(glyph, '-'); dash != -1 {
			first, err := strconv.ParseInt(glyph[:dash], 10, 16)
			if err != nil {
				return fmt.Errorf("invalid glyph ID: %v", err)
			}
			last, err := strconv.ParseInt(glyph[dash+1:], 10, 16)
			if err != nil {
				return fmt.Errorf("invalid glyph ID: %v", err)
			}
			if last < first || first < 0 || 65535 < last {
				return fmt.Errorf("invalid glyph ID range: %d-%d\n", first, last)
			}
			for first != last+1 {
				glyphMap[uint16(first)] = true
				first++
			}
		} else {
			glyphID, err := strconv.ParseInt(glyph, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid glyph ID: %v", err)
			}
			if glyphID < 0 || 65535 < glyphID {
				return fmt.Errorf("invalid glyph ID: %v", glyphID)
			}
			glyphMap[uint16(glyphID)] = true
		}
	}

	// append characters
	for _, s := range cmd.Chars {
		prev := rune(-1)
		rangeChars := false
		for _, r := range s {
			if prev != -1 && r == '-' {
				rangeChars = true
			} else if rangeChars {
				for i := prev + 1; i <= r; i++ {
					glyphID := sfnt.GlyphIndex(i)
					if glyphID == 0 {
						Warning.Println("glyph not found:", string(i))
					} else {
						glyphMap[glyphID] = true
					}
				}
				rangeChars = false
				prev = -1
			} else {
				glyphID := sfnt.GlyphIndex(r)
				if glyphID == 0 {
					Warning.Println("glyph not found:", string(r))
				} else {
					glyphMap[glyphID] = true
				}
				prev = r
			}
		}
		if rangeChars {
			glyphID := sfnt.GlyphIndex('-')
			if glyphID == 0 {
				Warning.Println("glyph not found: -")
			} else {
				glyphMap[glyphID] = true
			}
		}
	}

	// append glyph names
	for _, name := range cmd.Names {
		glyphID := sfnt.FindGlyphName(name)
		if glyphID == 0 {
			Warning.Println("glyph name not found:", name)
		} else {
			glyphMap[glyphID] = true
		}
	}

	// append unicode
	for _, code := range cmd.Unicodes {
		if dash := strings.IndexByte(code, '-'); dash != -1 {
			first, err := strconv.ParseInt(code[:dash], 16, 32)
			if err != nil {
				return fmt.Errorf("invalid unicode codepoint: %v", err)
			}
			last, err := strconv.ParseInt(code[dash+1:], 16, 32)
			if err != nil {
				return fmt.Errorf("invalid unicode codepoint: %v", err)
			}
			if last < first || first < 0 {
				return fmt.Errorf("invalid unicode range: U+%4X-U+%4X\n", first, last)
			}
			for first != last+1 {
				glyphID := sfnt.GlyphIndex(rune(first))
				if glyphID == 0 {
					Warning.Printf("glyph not found for U+%4X\n", first)
				} else {
					glyphMap[glyphID] = true
				}
				first++
			}
		} else {
			codepoint, err := strconv.ParseInt(code, 16, 32)
			if err != nil {
				return fmt.Errorf("invalid unicode codepoint: %v", err)
			} else if codepoint < 0 {
				return fmt.Errorf("invalid unicode codepoint: U+%4X\n", codepoint)
			}
			glyphID := sfnt.GlyphIndex(rune(codepoint))
			if glyphID == 0 {
				Warning.Printf("glyph not found for U+%4X\n", codepoint)
			} else {
				glyphMap[glyphID] = true
			}
		}
	}

	// append unicode ranges
	for _, unicodeRange := range cmd.UnicodeRanges {
		var ok bool
		var table *unicode.RangeTable
		if table, ok = unicode.Categories[unicodeRange]; !ok {
			if table, ok = unicode.Scripts[unicodeRange]; !ok {
				return fmt.Errorf("invalid unicode range: %v", unicodeRange)
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

	// set glyph names
	if sfnt.IsCFF && cmd.GlyphName == "" {
		sfnt.CFF.SetGlyphNames(nil)
	}

	// subset font
	numGlyphs := sfnt.NumGlyphs()
	sfntSubset, err := sfnt.Subset(glyphIDs, font.SubsetOptions{Tables: font.KeepMinTables})
	if err != nil {
		if cmd.Input == "-" {
			return err
		}
		return fmt.Errorf("%v: %v", cmd.Input, err)
	}

	// set glyph names
	if cmd.GlyphName != "" {
		names := make([]string, len(glyphIDs))
		for i, glyphID := range glyphIDs {
			name, ok := fmtName(cmd.GlyphName, sfnt, glyphID)
			if !ok {
				Warning.Println("missing glyph name or unicode mapping for glyph: %s(%d)", sfnt.GlyphName(glyphID), glyphID)
			} else {
				names[i] = name
			}
		}
		if err := sfntSubset.SetGlyphNames(names); err != nil {
			return fmt.Errorf("glyph names: %v", err)
		}
	}

	// create font program
	for _, output := range cmd.Outputs {
		mimetype := extMimetype[filepath.Ext(output)]
		if cmd.Type != "" {
			mimetype = cmd.Type
		} else if mimetype == "" {
			mimetype = rMimetype
		}
		wLen, err := writeFont(output, mimetype, cmd.Encoding, cmd.Force, sfntSubset)
		if err != nil {
			return err
		}

		ratio := 1.0
		if 0 < rLen {
			ratio = float64(wLen) / float64(rLen)
		}
		if !cmd.Quiet && output != "-" {
			numGlyphsSubset := sfntSubset.NumGlyphs()
			fmt.Printf("%v:  %v => %v glyphs,  %v => %v (%.1f%%)\n", filepath.Base(output), numGlyphs, numGlyphsSubset, formatBytes(uint64(rLen)), formatBytes(uint64(wLen)), ratio*100.0)
		}
	}
	return nil
}
