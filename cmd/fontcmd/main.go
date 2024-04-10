package main

import (
	"log"
	"os"

	"github.com/tdewolff/argp"
)

var (
	Error   *log.Logger
	Warning *log.Logger
)

func main() {
	Error = log.New(os.Stderr, "ERROR: ", 0)
	Warning = log.New(os.Stderr, "WARNING: ", 0)

	cmd := argp.New("Command line toolkit for TTF and OTF files - Taco de Wolff")
	cmd.AddCmd(&Info{}, "info", "Get font info")
	cmd.AddCmd(&Show{}, "draw", "Draw glyphs in terminal or output to image")
	cmd.AddCmd(&Subset{}, "subset", "Subset fonts")
	cmd.AddCmd(&CSS{}, "css", "Create CSS file")
	cmd.AddCmd(&Merge{}, "merge", "Merge fonts")
	cmd.Parse()
}
