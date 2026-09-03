package evtx

import (
	"strconv"

	"github.com/davecgh/go-spew/spew"
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

type HexInt uint64

func (i HexInt) String() string {
	return "0x" + strconv.FormatUint(uint64(i), 16)
}
