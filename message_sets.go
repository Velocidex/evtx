package evtx

import (
	"regexp"
	"strconv"
	"sync"
)

var (
	expansionRegex = regexp.MustCompile("%[0-9]+")
)

type MessageSet struct {
	mu         sync.Mutex
	Provider   string
	Channel    string
	Messages   map[int]string
	Parameters map[int]string
}

func (self *MessageSet) AddMessage(event_id int, message string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	number_of_expansions := self.getLargestExpansion(message)
	key := event_id<<16 | number_of_expansions

	self.Messages[key] = message
}

func (self *MessageSet) AddParameter(event_id int, message string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Parameters[event_id] = message
}

func (self *MessageSet) GetParameter(id int) string {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, _ := self.Parameters[id]
	return res
}

// Calculates the largest expansion number from the message string.
func (self *MessageSet) getLargestExpansion(message string) int {
	res := 0

	for _, m := range expansionRegex.FindAllString(message, -1) {
		val, err := strconv.Atoi(m[1:])
		if err == nil {
			val--
			if val > res {
				res = val
			}
		}
	}

	return res
}

// Sometimes a number of message strings are generated for each event
// id. This function finds the most appropriate message string with
// the most expansions relevant for this event.
func (self *MessageSet) GetBestMessage(
	event_id, number_of_expansions int) string {
	self.mu.Lock()
	defer self.mu.Unlock()

	for i := number_of_expansions; i > 0; i-- {
		key := event_id<<16 | i
		res, pres := self.Messages[key]
		if pres {
			return res
		}
	}
	return ""
}
