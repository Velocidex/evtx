//go:build windows
// +build windows

package evtx

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
