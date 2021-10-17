package evtx

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/Velocidex/ordereddict"
)

var (
	expansion_re    = regexp.MustCompile(`\%[0-9ntr]+`)
	parameter_re    = regexp.MustCompile(`^\%\%([0-9]+)`)
	system_root_re  = regexp.MustCompile("(?i)%?SystemRoot%?")
	windir_re       = regexp.MustCompile("(?i)%windir%")
	programfiles_re = regexp.MustCompile("(?i)%programfiles%")
	system32_re     = regexp.MustCompile(`(?i)\\System32\\`)
)

type MessageResolver interface {
	GetMessage(provider, channel string, event_id int) string
	GetParameter(provider, channel string, parameter_id int) string
	Close()
}

type NullResolver struct{}

func (self NullResolver) GetMessage(provider, channel string, event_id int) string {
	return ""
}

func (self NullResolver) GetParameter(provider, channel string, parameter_id int) string {
	return ""
}

func (self NullResolver) Close() {}

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

func maybeExpandObjects(provider, channel string,
	item interface{}, resolver MessageResolver) string {
	item_str, ok := item.(string)
	if !ok {
		return fmt.Sprintf("%v", item)
	}

	matches := parameter_re.FindStringSubmatch(item_str)
	if len(matches) < 2 {
		return fmt.Sprintf("%v", item)
	}

	param_id, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Sprintf("%v", item)
	}

	return resolver.GetParameter(provider, channel, param_id)
}

func ExpandMessage(
	event *ordereddict.Dict, resolver MessageResolver) string {

	provider, _ := ordereddict.GetString(event, "System.Provider.Name")
	provider_guid, _ := ordereddict.GetString(event, "System.Provider.Guid")
	channel, _ := ordereddict.GetString(event, "System.Channel")
	event_id, _ := ordereddict.GetInt(event, "System.EventID.Value")

	// Get the raw message. First try using the GUID then using the
	// name if possible.
	message := resolver.GetMessage(provider_guid, channel, event_id)
	if message == "" {
		message = resolver.GetMessage(provider, channel, event_id)
		if message == "" {
			// No raw message string, just return.
			return message
		}

	} else {
		provider = provider_guid
	}

	// Now get and flatten the user data or event data
	data, pres := ordereddict.GetMap(event, "UserData")
	if !pres {
		data, pres = ordereddict.GetMap(event, "EventData")
		if !pres {
			return message
		}
	}
	expansions := flatten(data)

	// Replace expansions in the message with the user data.
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
			return maybeExpandObjects(
				provider, channel, expansions[number], resolver)
		}
		return match
	})
}
