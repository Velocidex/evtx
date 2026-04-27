package evtx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	Filenames  map[string]int
}

func (self *MessageSet) Debug() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	res := ""
	for k, msg := range self.Messages {
		res += fmt.Sprintf("   %#x %v\n", k, strings.TrimSpace(msg))
	}

	return fmt.Sprintf("Provider: %v, Channel %v, Messages %v, Filenames %v:\n%v",
		self.Provider, self.Channel, len(self.Messages), self.Filenames, res)
}

func (self *MessageSet) AddMessage(
	event_id int, message, filename string) {

	if len(message) == 0 {
		return
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	// Sometimes we get several versions of the same message for the
	// same event id but different parameters. For example say EventID
	// X has 2 parameters sometimes, and 3 parameters some other
	// times. We need to be able to resolve the correct version of the
	// message depending on the number of parameters present.

	// To do this quickly, we shift the event id 16 bits to the left
	// and include the largest expansion in the bottom 16 bits. This
	// allows us to store different versions of messages for the same
	// event id, and also retrieve the correct message depending on
	// how the event is generated.
	number_of_expansions := self.getLargestExpansion(message)
	key := event_id<<16 | number_of_expansions

	// Only add the message if we do not already have it. This means
	// messages n files earlies in the search sequence will be found
	// instead of files later.
	_, pres := self.Messages[key]
	if pres {
		return
	}

	self.Messages[key] = message
	self.Filenames[filename] = 1
}

func (self *MessageSet) AddParameter(event_id int, message, filename string) {
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

	// Ideally we have the message which interpolates the most number
	// of expansions, but sometimes this is missing so we may have to
	// make do with a message that interpolates less
	// elements. Hopefully they are kind of related?
	for i := number_of_expansions; i >= 0; i-- {
		key := event_id<<16 | i
		res, pres := self.Messages[key]
		if pres {
			return res
		}
	}
	return ""
}
