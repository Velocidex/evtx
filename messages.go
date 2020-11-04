package evtx

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/Velocidex/ordereddict"
)

var (
	expansion_re    = regexp.MustCompile(`\%[0-9ntr]+`)
	system_root_re  = regexp.MustCompile("(?i)%?SystemRoot%?")
	windir_re       = regexp.MustCompile("(?i)%windir%")
	programfiles_re = regexp.MustCompile("(?i)%programfiles%")
	system32_re     = regexp.MustCompile(`(?i)\\System32\\`)
)

func flatten(dict *ordereddict.Dict) []interface{} {
	result := []interface{}{}

	for _, k := range dict.Keys() {
		value, _ := dict.Get(k)

		switch t := value.(type) {
		case *ordereddict.Dict:
			result = append(result, flatten(t)...)
		case []string:
			for _, item := range t {
				result = append(result, item)
			}
		default:
			result = append(result, value)
		}
	}

	return result
}

func ExpandMessage(event_map *ordereddict.Dict, message string) string {
	data, pres := ordereddict.GetMap(event_map, "UserData")
	if !pres {
		data, pres = ordereddict.GetMap(event_map, "EventData")
		if !pres {
			return message
		}
	}
	expansions := flatten(data)

	return expansion_re.ReplaceAllStringFunc(message, func(match string) string {
		switch match {
		case "%n":
			return "\n"
		case "%r":
			return ""
		case "%t":
			return "\t"
		}

		number, _ := strconv.Atoi(match[1:])

		// Regex expansions start at 1
		number -= 1
		if number >= 0 && number < len(expansions) {
			return fmt.Sprintf("%v", expansions[number])
		}
		return match
	})
}
