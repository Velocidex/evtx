package evtx

import (
	"fmt"
	"strconv"

	"github.com/davecgh/go-spew/spew"
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

type HexInt uint64

func (self HexInt) String() string {
	return "0x" + strconv.FormatUint(uint64(self), 16)
}

func (self HexInt) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, self.String())), nil
}
