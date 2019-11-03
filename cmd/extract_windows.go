// +build windows

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/windows/registry"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/binparsergen/reader"
	pe "www.velocidex.com/golang/go-pe"
)

var (
	extract      = app.Command("extract", "Extract all log messages from all providers.")
	extract_file = extract.Arg("file", "File to write all messages").Required().
			String()
)

// Walk over all the providers in the registry and call the callback
// with potential message files. The message_table paths are not
// guaranteed to exists.
func walkProvider(cb func(provider string, message_table string) error) error {
	channels_key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\EventLog`,
		registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer channels_key.Close()

	channels, err := channels_key.ReadSubKeyNames(-1)
	if err != nil {
		return err
	}

	for _, channel := range channels {
		providers_key, err := registry.OpenKey(channels_key, channel,
			registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
		if err != nil {
			return err
		}
		defer providers_key.Close()

		providers, err := providers_key.ReadSubKeyNames(-1)
		if err != nil {
			return err
		}

		for _, provider_name := range providers {
			one_provider_key, err := registry.OpenKey(providers_key, provider_name,
				registry.READ|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
			if err != nil {
				fmt.Printf("Unable to read provider %v\n", provider_name)
				continue
			}

			message_files, _, err := one_provider_key.GetStringValue("EventMessageFile")
			if err != nil {
				continue
			}

			for _, message_file := range expandLocations(message_files) {
				err = cb(provider_name, message_file)
				if err != nil {
					fmt.Printf("While processing %v (%v): %v\n",
						provider_name, message_file, err)
					continue
				}
			}
		}

	}

	return nil
}

var system_root_re = regexp.MustCompile("(?i)%?SystemRoot%?")
var windir_re = regexp.MustCompile("(?i)%windir%")
var programfiles_re = regexp.MustCompile("(?i)%programfiles%")
var system32_re = regexp.MustCompile(`(?i)\\System32\\`)

// Produce a list of possible locations the message file may be. We
// process all of them because sometimes event messages are split
// across multiple dlls. For example, a generic message table may
// exist in C:\Windows\System32\XXX.dll but a localized message table
// also exists in C:\Windows\System32\en-us\XXX.dll.mui
func expandLocations(message_file string) []string {

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

func makeDatabase(filename string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}

	_, err = database.Exec(`
    CREATE TABLE IF NOT EXISTS providers (
         id INTEGER PRIMARY KEY,
         name TEXT);

    CREATE TABLE IF NOT EXISTS messages (
         id INTEGER NOT NULL,
         event_id INTEGER NOT NULL,
         provider_id INTEGER NOT NULL,
         message TEXT
    );

    CREATE INDEX message_idx
    ON messages (event_id, provider_id);
`)
	if err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}

func doExtract() {
	providers := make(map[string]int64)

	handle, err := makeDatabase(*extract_file)
	kingpin.FatalIfError(err, "Can not open file %s: %v", *extract_file, err)

	defer handle.Close()

	get_provider_id, err := handle.Prepare("SELECT id from providers WHERE name = ?")
	kingpin.FatalIfError(err, " %v", err)
	defer get_provider_id.Close()

	insert_provider_id, err := handle.Prepare("INSERT INTO providers (name) values (?)")
	kingpin.FatalIfError(err, " %v", err)
	defer insert_provider_id.Close()

	insert_message, err := handle.Prepare("INSERT INTO messages (id, event_id, provider_id, message) VALUES (?, ?, ?, ?)")
	kingpin.FatalIfError(err, " %v", err)
	defer insert_message.Close()

	walkProvider(func(provider, message_table string) error {
		provider_id, pres := providers[provider]
		if !pres {
			rows, err := get_provider_id.Query(provider)
			kingpin.FatalIfError(err, "%v", err)

			defer rows.Close()

			i := 0
			for rows.Next() {
				rows.Scan(&provider_id)
				i += 1
			}

			if i == 0 {
				res, err := insert_provider_id.Exec(provider)
				if err != nil {
					return err
				}
				provider_id, _ = res.LastInsertId()
			}
			providers[provider] = provider_id
		}

		fd, err := os.Open(message_table)
		if err != nil {
			return nil
		}
		defer fd.Close()

		fmt.Printf("Openning message table file %v\n", message_table)

		reader, err := reader.NewPagedReader(fd, 4096, 100)
		if err != nil {
			return err
		}

		pe_file, err := pe.NewPEFile(reader)
		if err != nil {
			return err
		}

		messages := pe_file.GetMessages()
		if len(messages) > 10000 {
			return errors.New("Too many messages in dll")
		}
		for _, msg := range messages {
			message := strings.TrimSpace(msg.Message)
			_, err := insert_message.Exec(msg.Id, msg.EventId, provider_id, message)
			if err != nil {
				fmt.Printf("Err: %v %v %v\n", err, msg.EventId, provider_id)
			}
		}
		if len(messages) > 0 {
			fmt.Printf("Got %v messages for provider %v (%v) in %v\n",
				len(messages), provider, provider_id, message_table)
		}
		return nil
	})
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case extract.FullCommand():
			doExtract()

		default:
			return false
		}
		return true
	})
}
