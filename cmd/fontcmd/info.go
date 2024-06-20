package main

import (
	"fmt"
	"io/ioutil"
	"math"

	"github.com/tdewolff/font"
	"github.com/tdewolff/parse/v2"
)

type Info struct {
	Index   int    `short:"i" desc:"Font index for font collections"`
	Table   string `short:"t" desc:"OpenType table name"`
	GlyphID uint16 `short:"g" name:"glyph" desc:"Glyph ID"`
	Char    string `short:"c" desc:"Unicode character"`
	Output  string `short:"o" desc:"Output filename"`
	Input   string `index:"0" desc:"Input file"`
}

func (cmd *Info) Run() error {
	b, err := ioutil.ReadFile(cmd.Input)
	if err != nil {
		return err
	} else if b, err = font.ToSFNT(b); err != nil {
		return err
	}

	r := parse.NewBinaryReader(b)
	sfntVersion := r.ReadString(4)
	if sfntVersion == "ttcf" {
		_ = r.ReadUint32() // majorVersion and minorVersion
	}
	numTables := int(r.ReadUint16())
	_ = r.ReadBytes(6)

	version := "TrueType"
	if sfntVersion == "OTTO" {
		version = "CFF"
	} else if sfntVersion == "ttcf" {
		version = "Collection"
	}
	fmt.Printf("File: %s\n\n", cmd.Input)
	fmt.Printf("sfntVersion: 0x%08X (%s)\n", sfntVersion, version)
	fmt.Printf("\nTable directory:\n")

	nLen := int(math.Log10(float64(len(b))) + 1)
	for i := 0; i < numTables; i++ {
		tag := r.ReadString(4)
		checksum := r.ReadUint32()
		offset := r.ReadUint32()
		length := r.ReadUint32()
		fmt.Printf("  %2d  %s  checksum=0x%08X  offset=%*d  length=%*d\n", i, tag, checksum, nLen, offset, nLen, length)
	}
	return nil
}
