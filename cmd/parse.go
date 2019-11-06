package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
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
			event_map, ok := i.Event.(*ordereddict.Dict)
			if ok {
				event, ok := ordereddict.GetMap(event_map, "Event")
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

func (self *parsingContext) maybeExpandMessage(event_map *ordereddict.Dict) {
	// If not message database is loaded just ignore it.
	if self.query == nil {
		return
	}

	// Event.System.Provider.Name
	name, ok := ordereddict.GetString(event_map, "System.Provider.Name")
	if !ok {
		return
	}

	event_id, ok := ordereddict.GetInt(event_map, "System.EventID.Value")
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
			event_map.Set("Message", evtx.ExpandMessage(event_map, message))
			return
		}
	}
}

var expansion_re = regexp.MustCompile(`\%[0-9n]+`)

func (self *parsingContext) expandMessage(event_map *ordereddict.Dict, message string) string {
	expansions := []string{}

	data, pres := ordereddict.GetMap(event_map, "UserData.EventXML")
	if !pres {
		data_any, pres := ordereddict.GetAny(event_map, "EventData.Data")
		if !pres {
			return message
		}

		data_str, ok := data_any.([]string)
		if !ok {
			return message
		}

		expansions = data_str
		data = ordereddict.NewDict()
	}

	for _, key := range data.Keys() {
		if strings.HasPrefix(key, "xmlns") {
			continue
		}

		value, ok := data.Get(key)
		if ok {
			expansions = append(expansions, fmt.Sprintf("%v", value))
		}
	}

	return expansion_re.ReplaceAllStringFunc(message, func(match string) string {
		switch match {
		case "%n":
			return " "
		}

		number, _ := strconv.Atoi(match[1:])

		// Regex expansions start at 1
		number -= 1
		if number >= 0 && number < len(expansions) {
			return expansions[number]
		}
		return match
	})
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
