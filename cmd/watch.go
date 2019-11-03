package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/evtx"
)

var (
	watch      = app.Command("watch", "Watch a file for changes")
	watch_file = watch.Arg("file", "File to parse").Required().
			String()
)

func doWatch() {
	fd, err := os.OpenFile(*watch_file, os.O_RDONLY, os.FileMode(0666))
	kingpin.FatalIfError(err, "open file")

	open_file := func(fd *os.File) []*evtx.Chunk {
		chunks, err := evtx.GetChunks(fd)
		kingpin.FatalIfError(err, "Getting chunks")

		return chunks
	}

	max_record_id := uint64(0)

	// Now we want the file for events with record id larger than
	// this one.
	for {
		fmt.Printf("Will watch events newer than %v\n", max_record_id)

		new_max_record_id := max_record_id

		chunks := open_file(fd)
		for _, chunk := range chunks {
			end_of_chunk := chunk.Header.LastEventRecID
			if max_record_id > 0 && end_of_chunk > max_record_id {
				spew.Dump(chunk.Header)
				records, err := chunk.Parse(int(max_record_id))
				if err != nil {
					continue
				}

				// Display the records as json.
				for _, i := range records {
					serialized, _ := json.MarshalIndent(i.Event, " ", " ")
					fmt.Println(string(serialized))

					if i.Header.RecordID > new_max_record_id {
						new_max_record_id = i.Header.RecordID
					}
				}
			}
		}

		max_record_id = new_max_record_id
		time.Sleep(10 * time.Second)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case watch.FullCommand():
			doWatch()

		default:
			return false
		}
		return true
	})
}
