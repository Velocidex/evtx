// +build windows

package evtx

import (
	"os"
	"path/filepath"
	"strings"

	lru "github.com/hashicorp/golang-lru"
	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/binparsergen/reader"
	pe "www.velocidex.com/golang/go-pe"
)

func NewWindowsMessageResolver() *WindowsMessageResolver {
	cache, err := lru.New(100)
	if err != nil {
		panic(err)
	}
	return &WindowsMessageResolver{
		// string->MessageSet
		cache: cache,
	}
}

type WindowsMessageResolver struct {
	cache *lru.Cache
}

func (self *WindowsMessageResolver) getMessageSets(
	provider, channel string) (*MessageSet, error) {

	// Get provider from cache - the cache key is both provider and
	// channel.
	key := channel + provider
	message_set_any, pres := self.cache.Get(key)
	if !pres {
		var err error
		message_set_any, err = GetMessagesByGUID(provider, channel)
		if err != nil {
			// Try to get the messages by provider name
			message_set_any, err = GetMessages(provider, channel)
			if err != nil {
				// Cache the failure by storing nil in the map
				self.cache.Add(key, nil)
				return nil, err
			}
		}
		self.cache.Add(key, message_set_any)
	}

	// Negative cache
	if message_set_any == nil {
		return nil, errors.New("Not found")
	}

	return message_set_any.(*MessageSet), nil
}

func (self *WindowsMessageResolver) GetMessage(
	provider, channel string, event_id int) string {

	message_set, err := self.getMessageSets(provider, channel)
	if err != nil {
		return ""
	}

	// Get the event if it is there
	res, pres := message_set.Messages[event_id]
	if pres {
		return res.Message
	}
	return ""
}

func (self *WindowsMessageResolver) GetParameter(
	provider, channel string, parameter_id int) string {

	message_set, err := self.getMessageSets(provider, channel)
	if err != nil {
		return ""
	}

	if message_set.Parameters == nil {
		return ""
	}

	res, pres := message_set.Parameters[parameter_id]
	if pres {
		return res.Message
	}
	return ""
}

func (self *WindowsMessageResolver) Close() {}

type MessageSet struct {
	Provider   string
	Channel    string
	Messages   map[int]*pe.Message
	Parameters map[int]*pe.Message
}

// ExpandLocations Produces a list of possible locations the message
// file may be. We process all of them because sometimes event
// messages are split across multiple dlls. For example, a generic
// message table may exist in C:\Windows\System32\XXX.dll but a
// localized message table also exists in
// C:\Windows\System32\en-us\XXX.dll.mui
func ExpandLocations(message_file string) []string {

	// Expand environment variables in paths.
	replace_env_vars := func(paths []string) []string {
		system_root := os.Getenv("SystemRoot")
		windir := os.Getenv("WinDir")
		programfiles := os.Getenv("programfiles")
		programfiles_x86 := os.Getenv("ProgramFiles(x86)")

		result := []string{}
		for _, path := range paths {
			path = system_root_re.ReplaceAllLiteralString(
				path, system_root)

			path = windir_re.ReplaceAllLiteralString(
				path, windir)

			if programfiles_re.FindString(path) != "" {
				result = append(result,
					programfiles_re.ReplaceAllLiteralString(
						path, programfiles))
				result = append(result,
					programfiles_re.ReplaceAllLiteralString(
						path, programfiles_x86))
			} else {
				result = append(result, path)
			}
		}
		return result
	}

	// When paths refer to system32 the message table may instead
	// reside in the 32 bit version.
	split_system32 := func(paths []string) []string {
		result := []string{}
		for _, path := range paths {
			result = append(result, path)

			// Sometimes messages are found in the 32 bit folders.
			if system32_re.FindString(path) != "" {
				result = append(result, system32_re.ReplaceAllLiteralString(
					path, "\\SysWow64\\"))
			}
		}

		return result
	}

	include_muis := func(paths []string) []string {
		result := []string{}
		for _, path := range paths {
			result = append(result, path)

			// Sometimes messages are found in the MUI
			// files include those as well.
			dll_name := filepath.Base(path)
			dir_name := filepath.Dir(path)

			result = append(result, filepath.Join(
				dir_name, "en-US", dll_name+".mui"))
		}
		return result
	}

	// Message file values may be separated by ;
	return include_muis(split_system32(replace_env_vars(
		strings.Split(message_file, ";"))))
}

func GetMessagesByGUID(provider_guid, channel string) (*MessageSet, error) {
	key_path := `Software\Microsoft\Windows\CurrentVersion\WinEVT\Publishers\{` + provider_guid + "}"
	provider_key, err := registry.OpenKey(registry.LOCAL_MACHINE, key_path,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer provider_key.Close()

	message_files, _, err := provider_key.GetStringValue("MessageFileName")
	if err != nil {
		return nil, err
	}

	parameter_files, _, err := provider_key.GetStringValue("ParameterFileName")
	if err != nil {
		parameter_files = ""
	}

	provider, _, err := provider_key.GetStringValue("")
	if err != nil {
		provider = provider_guid
	}

	return expandLocations(message_files, parameter_files, provider, channel)
}

func expandLocations(
	message_files, parameter_files,
	provider, channel string) (*MessageSet, error) {
	result := &MessageSet{
		Provider:   provider,
		Channel:    channel,
		Messages:   make(map[int]*pe.Message),
		Parameters: make(map[int]*pe.Message),
	}

	populateMessages(message_files, result.Messages)
	if parameter_files != "" {
		populateMessages(parameter_files, result.Parameters)
	}

	return result, nil
}

func populateMessages(message_files string, set map[int]*pe.Message) {
	for _, message_file := range ExpandLocations(message_files) {
		fd, err := os.Open(message_file)
		if err != nil {
			continue
		}
		defer fd.Close()

		// fmt.Printf("Populating messages from %v\n", message_file)
		reader, err := reader.NewPagedReader(fd, 4096, 100)
		if err != nil {
			continue
		}

		pe_file, err := pe.NewPEFile(reader)
		if err != nil {
			continue
		}

		messages := pe_file.GetMessages()
		if len(messages) > 10000 {
			continue
		}

		for _, msg := range messages {
			set[msg.EventId] = msg
		}
	}
}

func GetMessages(provider, channel string) (*MessageSet, error) {
	root_key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\EventLog`,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer root_key.Close()

	channel_key, err := registry.OpenKey(root_key, channel,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer channel_key.Close()

	provider_key, err := registry.OpenKey(channel_key, provider,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer provider_key.Close()

	message_files, _, err := provider_key.GetStringValue("EventMessageFile")
	if err != nil {
		return nil, err
	}

	return expandLocations(message_files, "", provider, channel)
}
