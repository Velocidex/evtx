package evtx

import (
	"encoding/json"
	"fmt"
)

const (
	debug_enabled = false
)

func debug(format string, args ...interface{}) {
	if debug_enabled {
		fmt.Printf(format, args...)
	}
}

func DlvBreak() {}

func Dump(x interface{}) {
	serialized, _ := json.MarshalIndent(x, " ", " ")
	fmt.Println(string(serialized))
}
