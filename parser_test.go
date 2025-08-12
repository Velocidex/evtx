// This file tests the WindowsMessageResolver to make sure we can
// properly extract messages from the registry/files as we resolve the
// event.

package evtx

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
)

type EVTXTestSuite struct {
	suite.Suite
	binary string
}

func (self *EVTXTestSuite) SetupTest() {
	self.binary = "./dumpevtx"
	if runtime.GOOS == "windows" {
		self.binary += ".exe"
	}
}

func (self *EVTXTestSuite) TestCollector() {
	cmdline := []string{
		"parse", "--event_id", "4624",
		"--number", "1", "testdata/Security.evtx",
	}
	cmd := exec.Command(self.binary, cmdline...)
	out, err := cmd.CombinedOutput()
	assert.NoError(self.T(), err)

	out = bytes.ReplaceAll(out, []byte{'\r', '\n'}, []byte{'\n'})
	fixture_name := "Event4624_" + runtime.GOOS
	fmt.Printf("Testing fixture %v\n", fixture_name)

	goldie.Assert(self.T(), fixture_name, out)
}

func (self *EVTXTestSuite) TestTemplates() {
	cmdline := []string{
		"parse", "testdata/Microsoft-Windows-CAPI2_Operational_EventID70.evtx",
		"--disable_messages",
	}
	cmd := exec.Command(self.binary, cmdline...)
	out, err := cmd.CombinedOutput()
	assert.NoError(self.T(), err)

	out = bytes.ReplaceAll(out, []byte{'\r', '\n'}, []byte{'\n'})
	fixture_name := "CAPI2_Operational"
	fmt.Printf("Testing fixture %v\n", fixture_name)

	goldie.Assert(self.T(), fixture_name, out)
}

func TestEvtx(t *testing.T) {
	suite.Run(t, &EVTXTestSuite{})
}
