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

	// Resort the MUI cache to prefer a certain language (by default
	// English)
	new_mui_cache := make(map[string][]string)
	for k, list := range self.mui_cache {
		var new_list []string
		for _, item := range list {
			// If we match we push it to the front, otherwise we
			// append at the end. The result is that files matching
			// the regex are before ones that do not.
			if filter_re.MatchString(item) {
				new_list = append([]string{item}, new_list...)
			} else {
				new_list = append(new_list, item)
			}
		}
		new_mui_cache[k] = new_list
	}

	self.mui_cache = new_mui_cache

	return nil
}
