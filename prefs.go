//go:build windows
// +build windows

package evtx

import "regexp"

func (self *WindowsMessageResolver) sortMRUWithRegexp(filter string) error {
	if filter == "" {
		filter = "en-(US|AU|GB)"
	}

	filter_re, err := regexp.Compile("(?i)" + filter)
	if err != nil {
		return err
	}

	self.lang_filter_re = filter_re

	// Resort the MUI cache to prefer a certain language (by default
	// English)
	new_mui_cache := make(map[string][]string)
	for k, list := range self.mui_cache {
		new_mui_cache[k] = self.sortListWithPreference(list)
	}

	self.mui_cache = new_mui_cache

	return nil
}

// Reorder the list so files matching the language preference regex are
// before ones that do not.
func (self *WindowsMessageResolver) sortListWithPreference(list []string) []string {
	if self.lang_filter_re == nil {
		return list
	}

	var preferred, rest []string
	for _, item := range list {
		if self.lang_filter_re.MatchString(item) {
			preferred = append(preferred, item)
		} else {
			rest = append(rest, item)
		}
	}
	return append(preferred, rest...)
}
