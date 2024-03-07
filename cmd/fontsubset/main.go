package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

type CSSOptions struct {
	Output   string `desc:"CSS output file name."`
	Append   bool   `desc:"Append to the output file instead of overwriting."`
	Encoding string `desc:"Output encoding, either empty of base64."`
	Prefix   string `desc:"Glyph name prefix to use for CSS classes, eg. 'fa-'."`
}

func main() {
	// os.Exit doesn't execute pending defer calls, this is fixed by encapsulating run()
	os.Exit(run())
}

func run() int {
	glyphs := []string{}
	chars := []string{}
	names := []string{}
	unicodes := []string{}
	unicodeRanges := []string{}
	index := 0
	css := CSSOptions{}
	var encoding string
	var input, output string
	var quiet bool
	var force bool

	cmd := argp.New("Subset TTF/OTF/WOFF/WOFF2/EOT/TTC/OTC font file - Taco de Wolff")
	cmd.AddOpt(&quiet, "q", "quiet", "Suppress output except for errors.")
	cmd.AddOpt(&force, "f", "force", "Force overwriting existing files.")
	cmd.AddOpt(argp.Append{&glyphs}, "g", "glyph", "List of glyph IDs to keep, eg. 1-100.")
	cmd.AddOpt(argp.Append{&chars}, "c", "char", "List of literal characters to keep, eg. a-z.")
	cmd.AddOpt(argp.Append{&names}, "n", "name", "List of glyph names to keep, eg. space.")
	cmd.AddOpt(argp.Append{&unicodes}, "u", "unicode", "List of unicode IDs to keep, eg. f0fc-f0ff.")
	cmd.AddOpt(argp.Append{&unicodeRanges}, "r", "range", "List of unicode categories or scripts to keep, eg. L (for Letters) or Latin (latin script). See https://pkg.go.dev/unicode for all supported values.")
	cmd.AddOpt(&index, "", "index", "Index into font collection (used with TTC or OTC).")
	cmd.AddOpt(&encoding, "e", "encoding", "Output encoding, either empty of base64.")
	cmd.AddOpt(&css, "", "css", "")
	cmd.AddOpt(&output, "o", "output", "Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported).")
	cmd.AddArg(&input, "input", "Input font file.")
	cmd.Parse()

	Error := log.New(os.Stderr, "ERROR: ", 0)
	Warning := log.New(ioutil.Discard, "", 0)
	if !quiet {
		Warning = log.New(os.Stderr, "WARNING: ", 0)
	}

	if output == "" {
		output = input
	} else if encoding != "" && encoding != "base64" {
		Error.Println("unsupported encoding:", encoding)
		return 1
	} else if css.Output != "" && css.Encoding != "" && css.Encoding != "base64" {
		Error.Println("unsupported encoding for CSS:", encoding)
		return 1
	}

	// read from file and parse font
	var err error
	var r *os.File
	if input == "-" {
		r = os.Stdin
	} else if r, err = os.Open(input); err != nil {
		Error.Println(err)
		return 1
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		r.Close()
		Error.Println(err)
		return 1
	} else if err := r.Close(); err != nil {
		Error.Println(err)
		return 1
	}

	rLen := len(b)
	origExt := font.Extension(b)
	if b, err = font.ToSFNT(b); err != nil {
		Error.Println(err)
		return 1
	}

	sfnt, err := font.ParseSFNT(b, index)
	if err != nil {
		Error.Println(err)
		return 1
	}

	glyphMap := map[uint16]bool{}
	glyphMap[0] = true

	// append glyphs
	for _, glyph := range glyphs {
		if dash := strings.IndexByte(glyph, '-'); dash != -1 {
			first, err := strconv.ParseInt(glyph[:dash], 10, 16)
			if err != nil {
				Error.Println("invalid glyph ID:", err)
				return 1
			}
			last, err := strconv.ParseInt(glyph[dash+1:], 10, 16)
			if err != nil {
				Error.Println("invalid glyph ID:", err)
				return 1
			}
			if last < first || first < 0 || 65535 < last {
				Error.Printf("invalid glyph ID range: %d-%d\n", first, last)
				return 1
			}
			for first != last+1 {
				glyphMap[uint16(first)] = true
			}
		} else {
			glyphID, err := strconv.ParseInt(glyph, 10, 16)
			if err != nil {
				Error.Println("invalid glyph ID:", err)
				return 1
			}
			if glyphID < 0 || 65535 < glyphID {
				Error.Println("invalid glyph ID:", glyphID)
				return 1
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
	for _, name := range names {
		glyphID := sfnt.FindGlyphName(name)
		if glyphID == 0 {
			Warning.Println("glyph name not found:", name)
		} else {
			glyphMap[glyphID] = true
		}
	}

	// append unicode
	for _, code := range unicodes {
		if dash := strings.IndexByte(code, '-'); dash != -1 {
			first, err := strconv.ParseInt(code[:dash], 16, 32)
			if err != nil {
				Error.Println("invalid unicode codepoint:", err)
				return 1
			}
			last, err := strconv.ParseInt(code[dash+1:], 16, 32)
			if err != nil {
				Error.Println("invalid unicode codepoint:", err)
				return 1
			}
			if last < first || first < 0 {
				Error.Printf("invalid unicode range: U+%4X-U+%4X\n", first, last)
				return 1
			}
			for first != last+1 {
				glyphID := sfnt.GlyphIndex(rune(first))
				if glyphID == 0 {
					Warning.Printf("glyph not found for U+%4X\n", first)
				} else {
					glyphMap[glyphID] = true
				}
			}
		} else {
			codepoint, err := strconv.ParseInt(code, 16, 32)
			if err != nil {
				Error.Println("invalid unicode codepoint:", err)
				return 1
			} else if codepoint < 0 {
				Error.Printf("invalid unicode codepoint: U+%4X\n", codepoint)
				return 1
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
	for _, unicodeRange := range unicodeRanges {
		var ok bool
		var table *unicode.RangeTable
		if table, ok = unicode.Categories[unicodeRange]; !ok {
			if table, ok = unicode.Scripts[unicodeRange]; !ok {
				Error.Println("invalid unicode range:", unicodeRange)
				return 1
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

	// extract names for CSS classes
	var glyphNames []string
	var glyphRunes []rune
	if css.Output != "" {
		for _, glyphID := range glyphIDs {
			name := sfnt.GlyphName(glyphID)
			r := sfnt.Cmap.ToUnicode(glyphID)
			if name != "" && r != 0 {
				glyphNames = append(glyphNames, name)
				glyphRunes = append(glyphRunes, r)
			}
		}
	}

	// subset font
	numGlyphs := sfnt.NumGlyphs()
	sfnt = sfnt.Subset(glyphIDs, font.SubsetOptions{Tables: font.KeepMinTables})

	// create font program
	ext := filepath.Ext(output)
TryExtension:
	switch ext {
	case ".ttf", ".ttc":
		if sfnt.IsCFF {
			Error.Println("cannot convert CFF to TrueType glyph outlines")
			return 1
		}
		b = sfnt.Write()
	case ".otf", ".otc":
		if sfnt.IsTrueType {
			Error.Println("cannot convert TrueType to CFF glyph outlines")
			return 1
		}
		b = sfnt.Write()
	case ".woff2":
		if b, err = sfnt.WriteWOFF2(); err != nil {
			Error.Println(err)
			return 1
		}
	default:
		if ext != origExt {
			ext = origExt
			goto TryExtension
		}
		Error.Println("unsupported output file extension:", ext)
		return 1
	}
	wLen := len(b)
	ratio := 1.0
	if 0 < rLen {
		ratio = float64(wLen) / float64(rLen)
	}
	if !quiet && input != "-" {
		fmt.Printf("%v:  %v => %v glyphs,  %v => %v (%.1f%%)\n", filepath.Base(input), numGlyphs, len(glyphIDs), formatBytes(uint64(rLen)), formatBytes(uint64(wLen)), ratio*100.0)
	}

	// apply encoding
	if encoding == "base64" {
		dst := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
		base64.StdEncoding.Encode(dst, b)
		b = dst
	}

	// write to file
	var w io.WriteCloser
	if output == "-" {
		w = os.Stdout
	} else {
		if _, err := os.Stat(output); err == nil {
			if !force && !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", output), false) {
				return 0
			}
		}
		if w, err = os.Create(output); err != nil {
			Error.Println(err)
			return 1
		}
	}

	if _, err := w.Write(b); err != nil {
		w.Close()
		Error.Println(err)
		return 1
	} else if err := w.Close(); err != nil {
		Error.Println(err)
		return 1
	}

	if css.Output != "" {
		if css.Append {
			if w, err = os.OpenFile(css.Output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644); err != nil {
				Error.Println(err)
				return 1
			}
		} else {
			if _, err := os.Stat(css.Output); err == nil {
				if !force && !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", css.Output), false) {
					return 0
				}
			}
			if w, err = os.Create(css.Output); err != nil {
				Error.Println(err)
				return 1
			}
		}

		// apply encoding
		if encoding == "base64" {
			w = base64.NewEncoder(base64.StdEncoding, w)
		}

		// write css classes
		b := bufio.NewWriter(w)
		for i, r := range glyphRunes {
			fmt.Fprintf(b, ".%s%s:before{content:\"\\%x\"", css.Prefix, glyphNames[i], r)
		}
		if err := b.Flush(); err != nil {
			w.Close()
			Error.Println(err)
			return 1
		} else if err := w.Close(); err != nil {
			Error.Println(err)
			return 1
		}
	}
	return 0
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
