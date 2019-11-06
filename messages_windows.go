// +build windows

package evtx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/binparsergen/reader"
	pe "www.velocidex.com/golang/go-pe"
)

type MessageSet struct {
	Provider string
	Channel  string
	Messages map[int]*pe.Message
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

func GetMessages(provider, channel string) (*MessageSet, error) {
	result := &MessageSet{
		Provider: provider,
		Channel:  channel,
		Messages: make(map[int]*pe.Message),
	}
	provider_key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		fmt.Sprintf(`SYSTEM\CurrentControlSet\Services\EventLog\%s\%s`,
			channel, provider),
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer provider_key.Close()

	message_files, _, err := provider_key.GetStringValue("EventMessageFile")
	if err != nil {
		return nil, err
	}

	for _, message_file := range ExpandLocations(message_files) {
		fd, err := os.Open(message_file)
		if err != nil {
			continue
		}
		defer fd.Close()

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
			result.Messages[msg.EventId] = msg
		}
	}

	return result, nil
}
