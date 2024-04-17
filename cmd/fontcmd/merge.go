package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/tdewolff/font"
)

type Merge struct {
	Quiet         bool     `short:"q" desc:"Suppress output except for errors."`
	Force         bool     `short:"f" desc:"Force overwriting existing files."`
	Type          string   `short:"t" desc:"Explicitly set output mimetype, eg. font/woff2."`
	Encoding      string   `short:"e" desc:"Output encoding, either empty of base64."`
	RearrangeCmap bool     `desc:"Rearrange glyph unicode mapping, assigning a sequential codepoint for each glyph in order starting at 33 (exclamation)."`
	Outputs       []string `short:"o" desc:"Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported). Can output multiple file."`
	Inputs        []string `index:"*" desc:"Input font files."`
}

func (cmd *Merge) Run() error {
	if cmd.Quiet {
		Warning = log.New(ioutil.Discard, "", 0)
	}

	if len(cmd.Inputs) == 0 {
		return fmt.Errorf("input file names not set")
	} else if len(cmd.Outputs) == 0 {
		return fmt.Errorf("output file names not set")
	} else if cmd.Encoding != "" && cmd.Encoding != "base64" {
		return fmt.Errorf("unsupported encoding: %v", cmd.Encoding)
	}

	// read and parse from files and merge fonts
	sfnt, _, rLen, err := readFont(cmd.Inputs[0], 0)
	if err != nil {
		return err
	}

	for _, input := range cmd.Inputs[1:] {
		options := font.MergeOptions{
			RearrangeCmap: cmd.RearrangeCmap,
		}
		sfnt2, _, rLen2, err := readFont(input, 0)
		if err != nil {
			if input == "-" {
				return err
			}
			return fmt.Errorf("%v: %v", input, err)
		} else if err := sfnt.Merge(sfnt2, options); err != nil {
			if input == "-" {
				return err
			}
			return fmt.Errorf("%v: %v", input, err)
		}
		rLen += rLen2
	}

	// write merged font program
	for _, output := range cmd.Outputs {
		mimetype := extMimetype[filepath.Ext(output)]
		if cmd.Type != "" {
			mimetype = cmd.Type
		}
		wLen, err := writeFont(output, mimetype, cmd.Encoding, cmd.Force, sfnt)
		if err != nil {
			return err
		}

		ratio := 1.0
		if 0 < rLen {
			ratio = float64(wLen) / float64(rLen)
		}
		if !cmd.Quiet && output != "-" {
			fmt.Printf("%v:  %v => %v (%.1f%%)\n", filepath.Base(output), formatBytes(uint64(rLen)), formatBytes(uint64(wLen)), ratio*100.0)
		}
	}
	return nil
}
