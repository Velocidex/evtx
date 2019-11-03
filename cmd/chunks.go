package main

import (
	"os"

	"github.com/davecgh/go-spew/spew"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/evtx"
)

var (
	chunks      = app.Command("chunks", "Show the chunks in the file.")
	chunks_file = chunks.Arg("file", "File to parse").Required().
			OpenFile(os.O_RDONLY, os.FileMode(0666))
)

func doChunks() {
	chunks, err := evtx.GetChunks(*chunks_file)
	kingpin.FatalIfError(err, "Getting chunks")

	for _, c := range chunks {
		spew.Dump(c.Header)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case chunks.FullCommand():
			doChunks()

		default:
			return false
		}
		return true
	})
}
