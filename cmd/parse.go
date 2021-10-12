package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/evtx"
)

var (
	parse      = app.Command("parse", "Parse the events in the file.")
	parse_file = parse.Arg("file", "File to parse").Required().
			OpenFile(os.O_RDONLY, os.FileMode(0666))

	parse_output_file = parse.Flag("output", "File to write json in").
				OpenFile(os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(0666))

	parse_file_message_file = parse.Flag("messagedb", "Path to messages database.").
				String()

	start_record_id   = parse.Flag("start", "First EventID to dump").Int()
	number_of_records = parse.Flag("number", "How many records to print").
				Default("99999999").Int()

	event_id_filter = parse.Flag("event_id", "Only show these event IDs").Int()
)

type parsingContext struct {
	resolver evtx.MessageResolver
}

func (self *parsingContext) Parse() {
	chunks, err := evtx.GetChunks(*parse_file)
	kingpin.FatalIfError(err, "Getting chunks")

	count := 0
	for _, chunk := range chunks {
		records, err := chunk.Parse(*start_record_id)
		kingpin.FatalIfError(err, "Parsing chunk")

		for _, i := range records {
			event_map, ok := i.Event.(*ordereddict.Dict)
			if ok {
				event, ok := ordereddict.GetMap(event_map, "Event")
				if !ok {
					continue
				}

				// Filter by event id
				if *event_id_filter > 0 {
					event_id, _ := ordereddict.GetInt(event, "System.EventID.Value")
					if event_id != *event_id_filter {
						continue
					}
				}

				if self.resolver != nil {
					event.Set("Message", evtx.ExpandMessage(event, self.resolver))
				}

				// Quit after printing this many records.
				count++
				if count > *number_of_records {
					return
				}
				serialized, _ := json.MarshalIndent(event, " ", " ")
				if *parse_output_file == nil {
					fmt.Println(string(serialized))
				} else {
					(*parse_output_file).Write(serialized)
				}
			}
		}
	}
}

func NewParsingContext() *parsingContext {
	if *parse_file_message_file != "" {
		resolver, err := evtx.NewDBResolver(*parse_file_message_file)
		kingpin.FatalIfError(err, " %v", err)
		return &parsingContext{resolver}
	}

	// Otherwise use the native resolver
	resolver, err := evtx.GetNativeResolver()
	kingpin.FatalIfError(err, " %v", err)

	return &parsingContext{resolver}
}

func doParse() {
	ctx := NewParsingContext()
	ctx.Parse()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case parse.FullCommand():
			doParse()

		default:
			return false
		}
		return true
	})
}
