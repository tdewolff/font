package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/tdewolff/prompt"
)

type CSS struct {
	Quiet    bool   `short:"q" desc:"Suppress output except for errors."`
	Force    bool   `short:"f" desc:"Force overwriting existing files."`
	Selector string `desc:"Glyph name selector to use for CSS. Available variables: %i glyph ID, %n glyph name, %u glyph unicode in hexadecimal."`
	Encoding string `desc:"Output encoding, either empty of base64."`
	Append   bool   `short:"a" desc:"Append to the output file instead of overwriting."`
	Index    int    `short:"i" desc:"Index into font collection (used with TTC or OTC)."`
	Output   string `short:"o" desc:"CSS output file name."`
	Input    string `index:"0" desc:"Input font file."`
}

func (cmd *CSS) Run() error {
	if cmd.Quiet {
		Warning = log.New(ioutil.Discard, "", 0)
	}

	if cmd.Output == "" {
		return fmt.Errorf("output file name not set")
	} else if cmd.Encoding != "" && cmd.Encoding != "base64" {
		return fmt.Errorf("unsupported encoding for CSS: %v", cmd.Encoding)
	}

	// read from file and parse font
	sfnt, _, _, err := readFont(cmd.Input, cmd.Index)
	if err != nil {
		return err
	}

	// write CSS file
	var w io.WriteCloser
	if cmd.Append {
		if w, err = os.OpenFile(cmd.Output, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644); err != nil {
			return err
		}
	} else {
		if _, err := os.Stat(cmd.Output); err == nil {
			if !cmd.Force && !prompt.YesNo(fmt.Sprintf("%s already exists, overwrite?", cmd.Output), false) {
				return nil
			}
		}
		if w, err = os.Create(cmd.Output); err != nil {
			return err
		}
	}

	// apply encoding
	if cmd.Encoding == "base64" {
		w = base64.NewEncoder(base64.StdEncoding, w)
	}

	// write CSS classes
	b := bufio.NewWriter(w)
	for glyphID := uint16(1); glyphID < sfnt.NumGlyphs(); glyphID++ {
		r := sfnt.Cmap.ToUnicode(glyphID)
		name, ok := fmtName(cmd.Selector, sfnt, glyphID)
		if !ok || r == 0 {
			Warning.Println("could not write CSS class due to missing glyph name or unicode mapping for glyph ID", glyphID)
		} else {
			fmt.Fprintf(b, "%s{content:\"\\%x\"}\n", name, r)
		}
	}
	if err := b.Flush(); err != nil {
		w.Close()
		return err
	} else if err := w.Close(); err != nil {
		return err
	}
	return nil
}
