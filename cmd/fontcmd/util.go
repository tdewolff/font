package main

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"unicode"

	"github.com/tdewolff/font"
	"github.com/tdewolff/prompt"
)

var extMimetype = map[string]string{
	".ttf":   "font/truetype",
	".ttc":   "font/truetype",
	".otf":   "font/opentype",
	".otc":   "font/opentype",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".eot":   "font/eot",
}

func printableRune(r rune) string {
	if unicode.IsGraphic(r) {
		return fmt.Sprintf("%c", r)
	} else if r < 128 {
		return fmt.Sprintf("0x%02X", r)
	}
	return fmt.Sprintf("%U", r)
}

func printASCII(img image.Image) {
	palette := []byte("$@B%8&WM#*oahkbdpqwmZO0QLCJUYXzcvunxrjft/\\|()1{}[]?-_+~<>i!lI;:,\"^`'. ")

	size := img.Bounds().Max
	for j := 0; j < size.Y; j++ {
		for i := 0; i < size.X; i++ {
			r, g, b, _ := img.At(i, j).RGBA()
			y, _, _ := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			idx := int(float64(y)/255.0*float64(len(palette)-1) + 0.5)
			fmt.Print(string(palette[idx]))
		}
		fmt.Print("\n")
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

func fmtName(pattern string, sfnt *font.SFNT, glyphID uint16) (string, bool) {
	newName := &strings.Builder{}
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if i+1 < len(pattern) && c == '%' {
			i++
			switch pattern[i] {
			case '%':
				newName.WriteByte('%')
			case 'i':
				fmt.Fprintf(newName, "%d", glyphID)
			case 'n':
				glyphName := sfnt.GlyphName(glyphID)
				if glyphName == "" {
					return "", false
				}
				newName.WriteString(glyphName)
			case 'u':
				r := sfnt.Cmap.ToUnicode(glyphID)
				if r == 0 {
					return "", false
				}
				fmt.Fprintf(newName, "%x", r)
			}
		} else {
			newName.WriteByte(c)
		}
	}
	return newName.String(), true
}

func readFont(filename string, index int) (*font.SFNT, string, int, error) {
	var err error
	var r *os.File
	if filename == "-" {
		r = os.Stdin
	} else if r, err = os.Open(filename); err != nil {
		return nil, "", 0, err
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		r.Close()
		return nil, "", 0, err
	} else if err := r.Close(); err != nil {
		return nil, "", 0, err
	}

	n := len(b)
	mimetype, _ := font.MediaType(b)
	if b, err = font.ToSFNT(b); err != nil {
		return nil, "", 0, err
	}

	sfnt, err := font.ParseSFNT(b, index)
	if err != nil {
		return nil, "", 0, err
	}
	return sfnt, mimetype, n, nil
}

func writeFont(filename, mimetype, encoding string, force bool, sfnt *font.SFNT) (int, error) {
	var b []byte
	var err error
	switch mimetype {
	case "font/truetype":
		if sfnt.IsCFF {
			return 0, fmt.Errorf("cannot convert CFF to TrueType glyph outlines")
		}
		b = sfnt.Write()
	case "font/opentype":
		if sfnt.IsTrueType {
			return 0, fmt.Errorf("cannot convert TrueType to CFF glyph outlines")
		}
		b = sfnt.Write()
	case "font/woff2":
		if b, err = sfnt.WriteWOFF2(); err != nil {
			return 0, err
		}
	default:
		if mimetype == "" {
			return 0, fmt.Errorf("mimetype not set")
		}
		return 0, fmt.Errorf("unsupported output file type: %v", mimetype)
	}
	n := len(b)

	// apply encoding
	if encoding == "base64" {
		dst := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
		base64.StdEncoding.Encode(dst, b)
		b = dst
	}

	var w io.WriteCloser
	if filename == "-" {
		w = os.Stdout
	} else {
		if _, err := os.Stat(filename); err == nil {
			if !force && !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", filename), false) {
				return 0, fmt.Errorf("file already exists")
			}
		}
		if w, err = os.Create(filename); err != nil {
			return 0, err
		}
	}

	if _, err := w.Write(b); err != nil {
		w.Close()
		return 0, err
	} else if err := w.Close(); err != nil {
		return 0, err
	}
	return n, nil
}
