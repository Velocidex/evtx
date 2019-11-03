package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/evtx"
)

var (
	parse      = app.Command("parse", "Parse the events in the file.")
	parse_file = parse.Arg("file", "File to parse").Required().
			OpenFile(os.O_RDONLY, os.FileMode(0666))

	parse_file_message_file = parse.Flag("messagedb", "Path to messages database.").
				String()
	start_record_id = parse.Flag("start", "First EventID to dump").
			Int()
)

type parsingContext struct {
	db *sql.DB

	query *sql.Stmt
}

func NewParsingContext() *parsingContext {
	result := &parsingContext{}

	if *parse_file_message_file != "" {
		database, err := sql.Open("sqlite3", *parse_file_message_file)
		kingpin.FatalIfError(err, " %v", err)

		result.db = database

		result.query, err = database.Prepare(`
          SELECT message
          FROM messages left join providers ON messages.provider_id = providers.id
          WHERE providers.name = ? and messages.event_id = ?
               `)
		kingpin.FatalIfError(err, " %v", err)
	}

	return result
}

func (self *parsingContext) Parse() {
	chunks, err := evtx.GetChunks(*parse_file)
	kingpin.FatalIfError(err, "Getting chunks")

	for _, chunk := range chunks {
		records, err := chunk.Parse(*start_record_id)
		kingpin.FatalIfError(err, "Parsing chunk")

		for _, i := range records {
			event_map, ok := i.Event.(map[string]interface{})
			if ok {
				event, ok := GetMap(event_map, "Event")
				if !ok {
					continue
				}

				self.maybeExpandMessage(event)

				serialized, _ := json.MarshalIndent(event, " ", " ")
				fmt.Println(string(serialized))
			}
		}
	}
}

func (self *parsingContext) maybeExpandMessage(event_map map[string]interface{}) {
	// Event.System.Provider.Name
	name, ok := GetString(event_map, "System.Provider.Name")
	if !ok {
		return
	}

	event_id, ok := GetInt(event_map, "System.EventID.Value")
	if !ok {
		return
	}

	rows, err := self.query.Query(name, event_id)
	kingpin.FatalIfError(err, " %v", err)

	defer rows.Close()

	for rows.Next() {
		var message string
		err = rows.Scan(&message)
		if err == nil {
			event_map["Message"] = message
		}
	}
}

func GetString(event_map map[string]interface{}, members string) (string, bool) {
	var value interface{} = event_map
	var pres bool

	for _, member := range strings.Split(members, ".") {
		if event_map == nil {
			return "", false
		}

		value, pres = event_map[member]
		if !pres {
			return "", false
		}
		event_map, pres = value.(map[string]interface{})
	}

	value_str, ok := value.(string)
	if ok {
		return value_str, true
	}

	return "", false
}

func GetMap(event_map map[string]interface{}, members string) (map[string]interface{}, bool) {
	var value interface{} = event_map
	var pres bool

	for _, member := range strings.Split(members, ".") {
		if event_map == nil {
			return nil, false
		}

		value, pres = event_map[member]
		if !pres {
			return nil, false
		}
		event_map, pres = value.(map[string]interface{})
		if !pres {
			return nil, false
		}
	}

	return event_map, true
}

func GetInt(event_map map[string]interface{}, members string) (int, bool) {
	var value interface{} = event_map
	var pres bool

	for _, member := range strings.Split(members, ".") {
		if event_map == nil {
			return 0, false
		}

		value, pres = event_map[member]
		if !pres {
			return 0, false
		}
		event_map, pres = value.(map[string]interface{})
	}

	switch t := value.(type) {
	case int:
		return t, true
	case uint8:
		return int(t), true
	case uint16:
		return int(t), true
	case uint32:
		return int(t), true
	case uint64:
		return int(t), true
	case int8:
		return int(t), true
	case int16:
		return int(t), true
	case int32:
		return int(t), true
	case int64:
		return int(t), true

	}

	return 0, false
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
