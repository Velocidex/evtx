package evtx

import (
	"encoding/binary"
	"testing"
	"time"
)

// TestConsumeUint8AtBufferBoundary checks ConsumeUint8's bounds check at and
// around the exact end of the buffer. offset == len(buff) must return 0
// without panicking; a prior off-by-one guard (`if self.offset >
// len(self.buff)`) let self.buff[self.offset] run once past the end there.
func TestConsumeUint8AtBufferBoundary(t *testing.T) {
	const buflen = 8

	// The buffer is filled with a non-zero marker so the in-bounds case
	// asserts an actual read: with a zeroed buffer every case would expect 0,
	// and the test could not tell a correct read from a guard that bailed out
	// and returned the zero value.
	const marker = 0xAB

	cases := []struct {
		name   string
		offset int
		want   uint8
	}{
		{name: "last valid byte", offset: buflen - 1, want: marker},
		{name: "exactly at end", offset: buflen, want: 0},
		{name: "past end", offset: buflen + 1, want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chunk := &Chunk{}
			ctx := NewParseContext(chunk)
			ctx.buff = make([]byte, buflen)
			for i := range ctx.buff {
				ctx.buff[i] = marker
			}
			ctx.offset = c.offset

			got := ctx.ConsumeUint8()

			if got != c.want {
				t.Errorf("ConsumeUint8() at offset %d = %d, want %d", c.offset, got, c.want)
			}
		})
	}
}

// TestConsumeBytesOutOfBounds checks that ConsumeBytes returns nil, rather
// than allocating a stream-chosen size, when the requested size overruns the
// buffer. size is read straight from the stream at several call sites (e.g.
// an argument length), so allocating make([]byte, size) on that path lets a
// malformed record drive an attacker-chosen allocation instead of simply
// failing the read, as every other Consume* method already does by
// returning its zero value.
func TestConsumeBytesOutOfBounds(t *testing.T) {
	cases := []struct {
		name string
		buff []byte
		size int
	}{
		{name: "size exceeds remaining buffer", buff: make([]byte, 4), size: 5},
		{name: "size exceeds empty buffer", buff: nil, size: 1024 * 1024 * 1024},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chunk := &Chunk{}
			ctx := NewParseContext(chunk)
			ctx.buff = c.buff

			got := ctx.ConsumeBytes(c.size)

			if got != nil {
				t.Errorf("ConsumeBytes(%d) = %v (len %d), want nil", c.size, got, len(got))
			}
		})
	}
}

// buildForgedTemplateInstance returns the bytes ParseTemplateInstance reads
// for a template instance whose short_id has never been seen before (the
// !pres branch), with the second, template-body numArguments read forged to
// forgedNumArguments. This is the only branch reachable from a .evtx file
// forged from scratch, since a short_id cannot be "already known" without
// first being defined by this same branch.
func buildForgedTemplateInstance(forgedNumArguments uint32) []byte {
	buf := make([]byte, 40)
	buf[0] = 0x01
	binary.LittleEndian.PutUint32(buf[1:], 1) // short_id
	// buf[5:9]: template_definition_data, unused by this test.
	binary.LittleEndian.PutUint32(buf[9:], 5) // numArguments, 1st read: irrelevant.
	// buf[13:29]: 16-byte long GUID, skipped since short_id is not yet known.
	binary.LittleEndian.PutUint32(buf[29:], 0) // templateBodyLen = 0.
	binary.LittleEndian.PutUint32(buf[33:], forgedNumArguments)
	return buf
}

// TestParseTemplateInstanceCapsForgedNumArguments checks that a forged
// numArguments reaching the arg-header loop through the !pres branch cannot
// drive an unbounded number of append iterations. A cap that only applies to
// the first (fast-path) read of numArguments does not protect this branch,
// since it overwrites numArguments with a second, uncapped read.
func TestParseTemplateInstanceCapsForgedNumArguments(t *testing.T) {
	cases := []struct {
		name            string
		forgedArguments uint32
	}{
		{name: "near uint32 max", forgedArguments: 0xFFFFFFFE},
		{name: "just above the cap", forgedArguments: maxTemplateArguments + 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chunk := &Chunk{}
			ctx := NewParseContext(chunk)
			ctx.buff = buildForgedTemplateInstance(c.forgedArguments)

			done := make(chan bool, 1)
			go func() {
				done <- ParseTemplateInstance(ctx)
			}()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("ParseTemplateInstance did not return within 3s on a forged "+
					"numArguments=%#x via the !pres branch", c.forgedArguments)
			}
		})
	}
}
