package evtx

import "fmt"

const (
	debug_enabled = false
)

func debug(format string, args ...interface{}) {
	if debug_enabled {
		fmt.Printf(format, args...)
	}
}
