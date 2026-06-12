//go:build windows
// +build windows

package evtx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/binparsergen/reader"
	pe "www.velocidex.com/golang/go-pe"
)

var (
	// Search for potential MUI files - these are typically found in
	// directories names like " en-US cz-CZ
	mui_dir_regex   = regexp.MustCompile("^[a-z]{2}-[a-z]{2}$")
	system_root_re  = regexp.MustCompile("(?i)%?SystemRoot%?")
	windir_re       = regexp.MustCompile("(?i)%windir%")
	programfiles_re = regexp.MustCompile("(?i)%programfiles%")
	system32_re     = regexp.MustCompile(`(?i)\\System32\\`)

	invalidGUID = errors.New("invalidGUID")

	mui_debug = 0
)

// NewWindowsMessageResolver is the constructor for the
// WindowsMessageResolver.
func NewWindowsMessageResolver(
	opts MessageResolverOpts) (*WindowsMessageResolver, error) {
	lru_size := opts.LRUSize
	if lru_size <= 0 {
		lru_size = 100
	}

	cache, err := lru.New(lru_size)
	if err != nil {
		return nil, err
	}

	self := &WindowsMessageResolver{
		// string->MessageSet
		cache: cache,

		// MUI files can be found in the SxS directory - we cache that
		// periodically.
		mui_cache:        make(map[string][]string),
		checked_mui_dirs: make(map[string]bool),
		opts:             opts,
	}

	self.system_root = os.Getenv("SystemRoot")
	if self.system_root == "" {
		self.system_root = "C:/Windows/"
	}

	filter := opts.LangPreferenceRegeExp
	if filter == "" {
		filter = "en-(US|AU|GB)"
	}

	filter_re, err := regexp.Compile("(?i)" + filter)
	if err != nil {
		return nil, err
	}

	self.lang_filter_re = filter_re

	// Get MUI files from e.g. C:\Windows\System32\en-US\*.mui
	self.buildMUIcacheFromDir(
		filepath.Join(self.system_root, "System32"), mui_dir_regex)

	// Get MUI files from e.g. C:\Windows\WinSxS\*\*.mui
	// This seems to slow things down a lot because there are many SxS dlls typically.
	//self.buildMUIcacheFromDir(
	//	filepath.Join(system_root, "WinSxS"), nil)

	return self, nil
}

type WindowsMessageResolver struct {
	cache *lru.Cache

	// Protect the below
	mu               sync.Mutex
	mui_cache        map[string][]string
	checked_mui_dirs map[string]bool
	system_root      string
	lang_filter_re   *regexp.Regexp
	opts             MessageResolverOpts
}

// Discover possible MUI files in the directory specified.
func (self *WindowsMessageResolver) buildMUIcacheFromDir(
	directory string, dir_regex *regexp.Regexp) {

	// Only process directory once.
	_, pres := self.checked_mui_dirs[directory]
	if pres {
		return
	}
	self.checked_mui_dirs[directory] = true

	files, err := os.ReadDir(directory)
	if err != nil {
		return
	}

	for _, file := range files {
		filename := strings.ToLower(file.Name())
		if dir_regex != nil && !dir_regex.MatchString(filename) {
			continue
		}

		mui_dir := filepath.Join(directory, file.Name())
		files, err := os.ReadDir(mui_dir)
		if err != nil {
			continue
		}

		for _, file := range files {
			filename := strings.ToLower(file.Name())
			if !strings.HasSuffix(filename, ".mui") {
				continue
			}

			fullpath := filepath.Join(mui_dir, file.Name())

			basename := strings.TrimSuffix(filepath.Base(filename), ".mui")
			locations, _ := self.mui_cache[basename]

			// Make sure the mui_cache is always properly sorted.
			self.mui_cache[basename] = self.sortListWithPreference(
				append(locations, fullpath))

			if mui_debug > 0 {
				fmt.Printf("buildMUIcacheFromDir: adding %v to %v\n", fullpath, basename)
			}
		}
	}
}

// ExpandMessageFileLocation Produces a list of possible locations the
// message file may be. We process all of them because sometimes event
// messages are split across multiple dlls. For example, a generic
// message table may exist in C:\Windows\System32\XXX.dll but a
// localized message table also exists in
// C:\Windows\System32\en-us\XXX.dll.mui
//
// NOTE: This function will effectively be called once per provider
// since there is a higher level provider LRU cache. So it is probably
// not worth memoizing it.
func (self *WindowsMessageResolver) ExpandMessageFileLocation(
	message_file string) []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	windir := os.Getenv("WinDir")
	programfiles := os.Getenv("programfiles")
	programfiles_x86 := os.Getenv("ProgramFiles(x86)")

	// Expand environment variables in paths.
	replace_env_vars := func(paths []string) []string {
		result := []string{}
		for _, path := range paths {
			path = system_root_re.ReplaceAllLiteralString(
				path, self.system_root)

			path = windir_re.ReplaceAllLiteralString(path, windir)

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

	// On international systems messages may be stored in MUI
	// files. This appends possible MUI files **after** the provided
	// list. The search order looks at the provided list first
	// (usually System32) and then only if the message is not found
	// consults the MUI files.
	include_muis := func(paths []string) []string {
		result := []string{}
		seen := make(map[string]bool)

		for _, path := range paths {
			result = append(result, path)

			dirname := filepath.Dir(path)

			// Make sure we checked this directory for MUI
			// files. Sometimes an application will distribute MUI
			// files inside its own path.
			self.buildMUIcacheFromDir(dirname, mui_dir_regex)

			dll_name := strings.ToLower(filepath.Base(path))

			// process each dll only once to avoid recursion.
			_, pres := seen[dll_name]
			if pres {
				continue
			}
			seen[dll_name] = true

			// Append all muis in search order.
			muis, pres := self.mui_cache[dll_name]
			if pres {
				// muis list is always properly sorted in search order.
				result = append(result, muis...)
			}
		}
		return result
	}

	// Stat each file to ensure it exists
	filter_files := func(paths []string) []string {
		result := []string{}
		for _, path := range paths {
			_, err := os.Lstat(path)
			if err != nil {
				continue
			}
			result = append(result, path)
		}
		return result
	}

	locations := strings.Split(message_file, ";")
	return filter_files(
		include_muis(split_system32(replace_env_vars(locations))))
}

func (self *WindowsMessageResolver) GetMessageSets(
	provider, channel string) (*MessageSet, error) {

	// Get provider from cache - the cache key is both provider and
	// channel.
	key := channel + provider
	message_set_any, pres := self.cache.Get(key)
	if !pres {
		var err error
		message_set_any, err = self.GetMessagesByGUID(provider, channel)
		if err != nil {
			// Try to get the messages by provider name
			message_set_any, err = self.GetMessages(provider, channel)
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
	provider, channel string, event_id, number_of_expansions int) string {

	message_set, err := self.GetMessageSets(provider, channel)
	if err != nil {
		return ""
	}

	if mui_debug >= 2 {
		fmt.Printf("Getting event id %#x from %v (number_of_expansions %v)\n",
			event_id, message_set.Debug(), number_of_expansions)
	}
	// Get the event if it is there
	return message_set.GetBestMessage(event_id, number_of_expansions)
}

func (self *WindowsMessageResolver) GetParameter(
	provider, channel string, parameter_id int) string {

	message_set, err := self.GetMessageSets(provider, channel)
	if err != nil {
		return ""
	}

	if message_set.Parameters == nil {
		return ""
	}

	return message_set.GetParameter(parameter_id)
}

func (self *WindowsMessageResolver) Close() {}

func (self *WindowsMessageResolver) GetMessagesByGUID(
	provider_guid, channel string) (*MessageSet, error) {

	if mui_debug > 0 {
		fmt.Printf("GetMessagesByGUID %v: %v\n", provider_guid, channel)
	}

	if len(provider_guid) == 0 {
		return nil, invalidGUID
	}

	// Sometimes the provider_guid contains the {} and sometimes it does not?
	provider_guid = strings.Trim(provider_guid, "{}")
	key_path := fmt.Sprintf(
		`Software\Microsoft\Windows\CurrentVersion\WinEVT\Publishers\{%s}`,
		provider_guid)

	provider_key, err := registry.OpenKey(registry.LOCAL_MACHINE, key_path,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		if mui_debug > 0 {
			fmt.Printf("GetMessagesByGUID OpenKey %v: %v\n", key_path, err)
		}
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

	return self.GetMessageSetsForProvider(
		message_files, parameter_files, provider, channel)
}

func (self *WindowsMessageResolver) GetMessageSetsForProvider(
	message_files, parameter_files,
	provider, channel string) (*MessageSet, error) {
	msg_set := &MessageSet{
		Provider:   provider,
		Channel:    channel,
		Messages:   make(map[int]string),
		Parameters: make(map[int]string),
		Filenames:  make(map[string]int),
	}

	self.PopulateMessages(message_files, msg_set.AddMessage)
	if parameter_files != "" {
		self.PopulateMessages(parameter_files, msg_set.AddParameter)
	}

	return msg_set, nil
}

func (self *WindowsMessageResolver) PopulateMessages(
	message_files string,
	adder func(event_id int, message string, filename string)) {
	for _, message_file := range self.ExpandMessageFileLocation(
		message_files) {
		fd, err := os.Open(message_file)
		if err != nil {
			continue
		}
		defer fd.Close()

		if mui_debug > 1 {
			fmt.Printf("Populating messages from %v\n", message_file)
		}
		reader, err := reader.NewPagedReader(fd, 4096, 100)
		if err != nil {
			continue
		}

		if mui_debug > 1 {
			fmt.Printf("Loading PE file %v\n", message_file)
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
			adder(msg.EventId, msg.Message, message_file)
		}
	}
}

func (self *WindowsMessageResolver) GetMessages(
	provider, channel string) (*MessageSet, error) {
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

	if mui_debug > 1 {
		fmt.Printf("GetMessages %v %v: %v\n", provider, channel, message_files)
	}
	return self.GetMessageSetsForProvider(message_files, "", provider, channel)
}
