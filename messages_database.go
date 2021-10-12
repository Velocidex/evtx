package evtx

import (
	"database/sql"
)

type DBResolver struct {
	db    *sql.DB
	query *sql.Stmt
}

// TODO: What is happening with the channel here?
func (self *DBResolver) GetMessage(
	provider, channel string, event_id int) string {
	rows, err := self.query.Query(provider, event_id)
	if err != nil {
		return ""
	}

	defer rows.Close()

	for rows.Next() {
		var message string
		err = rows.Scan(&message)
		if err == nil {
			return message
		}
	}
	return ""
}

func (self *DBResolver) GetParameter(provider, channel string, parameter_id int) string {
	return ""
}

func (self *DBResolver) Close() {
	self.db.Close()
}

func NewDBResolver(message_file string) (*DBResolver, error) {
	database, err := sql.Open("sqlite3", message_file)
	if err != nil {
		return nil, err
	}
	query, err := database.Prepare(`
          SELECT message
          FROM messages left join providers ON messages.provider_id = providers.id
          WHERE providers.name = ? and messages.event_id = ?
               `)

	if err != nil {
		return nil, err
	}

	return &DBResolver{db: database, query: query}, nil
}
