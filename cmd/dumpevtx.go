/*
   Copyright 2018 Velocidex Innovations

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
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
	app = kingpin.New("dumpevtx",
		"A tool for dumping evtx files.")

	chunks      = app.Command("chunks", "Show the chunks in the file.")
	chunks_file = chunks.Arg("file", "File to parse").Required().
			OpenFile(os.O_RDONLY, os.FileMode(0666))

	parse      = app.Command("parse", "Parse the events in the file.")
	parse_file = parse.Arg("file", "File to parse").Required().
			OpenFile(os.O_RDONLY, os.FileMode(0666))
	start_record_id = parse.Flag("start", "First EventID to dump").
			Int()

	watch      = app.Command("watch", "Watch a file for changes")
	watch_file = watch.Arg("file", "File to parse").Required().
			String()
)

func doChunks() {
	chunks, err := evtx.GetChunks(*chunks_file)
	kingpin.FatalIfError(err, "Getting chunks")

	for _, c := range chunks {
		spew.Dump(c.Header)
	}
}

func doParse() {
	chunks, err := evtx.GetChunks(*parse_file)
	kingpin.FatalIfError(err, "Getting chunks")

	for _, chunk := range chunks {
		records, err := chunk.Parse(*start_record_id)
		kingpin.FatalIfError(err, "Parsing chunk")

		for _, i := range records {
			serialized, _ := json.MarshalIndent(i, " ", " ")
			fmt.Println(string(serialized))
		}
	}
}

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
					serialized, _ := json.MarshalIndent(i, " ", " ")
					fmt.Println(string(serialized))
				}
			}

			if end_of_chunk > new_max_record_id {
				new_max_record_id = chunk.Header.LastEventRecID
			}
		}

		max_record_id = new_max_record_id
		time.Sleep(10 * time.Second)
	}
}

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case chunks.FullCommand():
		doChunks()

	case parse.FullCommand():
		doParse()

	case watch.FullCommand():
		doWatch()

	default:
		fmt.Println("Try -h for help.")
	}
}
