package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	lookup      = app.Command("lookup", "Lookup log message.")
	lookup_file = lookup.Arg("file", "message database").Required().
			String()

	lookup_provider = lookup.Arg("provider", "Provider name").Required().String()
	lookup_eventid  = lookup.Arg("event_id", "Event ID").Required().Int64()
)

type Row struct {
	Id       int
	EventId  int
	Provider string
	Message  string
}

func doLookup() {
	database, err := sql.Open("sqlite3", *lookup_file)
	kingpin.FatalIfError(err, " %v", err)

	get_events, err := database.Prepare(`
          SELECT messages.id, providers.name, event_id, message
          FROM messages left join providers ON messages.provider_id = providers.id
          WHERE providers.name = ? and messages.event_id = ?
        `)
	kingpin.FatalIfError(err, " %v", err)
	defer get_events.Close()

	rows, err := get_events.Query(*lookup_provider, *lookup_eventid)
	kingpin.FatalIfError(err, "%v", err)

	defer rows.Close()

	for rows.Next() {
		r := &Row{}
		err := rows.Scan(&r.Id, &r.Provider, &r.EventId, &r.Message)
		kingpin.FatalIfError(err, "%v", err)

		fmt.Printf("%v %v %v %v\n", r.Id, r.EventId, r.Provider, r.Message)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case lookup.FullCommand():
			doLookup()

		default:
			return false
		}
		return true
	})
}
