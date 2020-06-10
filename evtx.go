/*
   Copyright 2018 Velocidex Innovations

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
package evtx

import (
	"bytes"
	"encoding/binary"
	"strings"

	"fmt"
	"io"
	"os"
	"unicode/utf16"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
)

const (
	EVTX_HEADER_MAGIC       = "ElfFile\x00"
	EVTX_CHUNK_HEADER_MAGIC = "ElfChnk\x00"
	EVTX_CHUNK_HEADER_SIZE  = 0x200

	EVTX_CHUNK_SIZE = 0x10000

	EVTX_EVENT_RECORD_MAGIC = "\x2a\x2a\x00\x00"
	EVTX_EVENT_RECORD_SIZE  = 24
)

type EvtxGUID struct {
	D  uint32
	W1 uint16
	W2 uint16
	B  [8]uint8
}

func (self *EvtxGUID) ToString() string {
	return fmt.Sprintf(
		"%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		self.D, self.W1, self.W2,
		self.B[0], self.B[1], self.B[2], self.B[3],
		self.B[4], self.B[5], self.B[6], self.B[7])
}

type EVTXHeader struct {
	Magic           [8]byte
	Firstchunk      uint64
	LastChunk       uint64
	NextRecordID    uint64
	HeaderSize      uint32
	MinorVersion    uint16
	MajorVersion    uint16
	HeaderBlockSize uint16
	_               [76]byte
	FileFlags       uint32
	CheckSum        uint32
}

type EventRecordHeader struct {
	Magic    [4]byte
	Size     uint32
	RecordID uint64
	FileTime uint64
}

type EventRecord struct {
	Header EventRecordHeader
	Event  interface{}
}

func (self *EventRecord) Parse(ctx *ParseContext) {
	template := ctx.NewTemplate(0)
	ParseBinXML(ctx)

	self.Event = template.Expand(nil)
}

func NewEventRecord(ctx *ParseContext, chunk *Chunk) (*EventRecord, error) {
	self := &EventRecord{}
	record_header := bytes.NewBuffer(ctx.ConsumeBytes(EVTX_EVENT_RECORD_SIZE))
	err := binary.Read(record_header, binary.LittleEndian, &self.Header)
	if err != nil {
		return nil, errors.Wrap(err, "Read failed")
	}

	if string(self.Header.Magic[:]) != EVTX_EVENT_RECORD_MAGIC {
		return nil, errors.New("Record does not have the right magic")
	}

	return self, nil
}

type ChunkHeader struct {
	Magic               [8]byte
	FirstEventRecNumber uint64
	LastEventRecNumber  uint64
	FirstEventRecID     uint64
	LastEventRecID      uint64
	HeaderSize          uint32
}

type Chunk struct {
	Header ChunkHeader
	Offset int64
	Fd     io.ReadSeeker
}

func (self *Chunk) Parse(start_record_id int) ([]*EventRecord, error) {
	result := []*EventRecord{}
	buf := make([]byte, EVTX_CHUNK_SIZE)
	_, err := self.Fd.Seek(self.Offset, os.SEEK_SET)
	if err != nil {
		return nil, errors.Wrap(err, "Seek")
	}

	_, err = io.ReadAtLeast(self.Fd, buf, EVTX_CHUNK_SIZE)
	if err != nil {
		return nil, errors.Wrap(err, "ReadAtLeast")
	}

	// The entire chunk is captured in this context.
	ctx := NewParseContext(self)
	ctx.buff = buf
	ctx.offset = EVTX_CHUNK_HEADER_SIZE

	for i := self.Header.FirstEventRecNumber; i <= self.Header.LastEventRecNumber; i++ {
		start_of_record := ctx.Offset()
		record, err := NewEventRecord(ctx, self)
		if err != nil {
			return result, nil
		}

		// We have to parse all the records in case they
		// define templates we need.
		record.Parse(ctx)
		if int(record.Header.RecordID) >= start_record_id {
			result = append(result, record)
		}

		ctx.SetOffset(start_of_record + int(record.Header.Size))
	}
	return result, nil
}

func NewChunk(fd io.ReadSeeker, offset int64) (*Chunk, error) {
	self := &Chunk{Offset: offset, Fd: fd}
	_, err := fd.Seek(offset, os.SEEK_SET)
	if err != nil {
		return nil, errors.Wrap(err, "Seek")
	}

	err = binary.Read(fd, binary.LittleEndian, &self.Header)
	return self, errors.WithStack(err)
}

type TemplateNode struct {
	Id          uint32
	Type        uint32
	Literal     interface{}
	NestedArray []*TemplateNode
	NestedDict  *ordereddict.Dict //map[string]*TemplateNode

	CurrentKey string
}

func (self *TemplateNode) Expand(args map[int]interface{}) interface{} {
	if self.NestedDict != nil {
		result := ordereddict.NewDict()
		for _, k := range self.NestedDict.Keys() {
			v, _ := self.NestedDict.Get(k)
			expanded := v.(*TemplateNode).Expand(args)
			if k == "" {
				k = "Value"
				if self.NestedDict.Len() == 1 {
					return expanded
				}

				expanded_dict, ok := expanded.(*ordereddict.Dict)
				if ok {
					for _, k := range expanded_dict.Keys() {
						v, _ := expanded_dict.Get(k)
						if v != nil {
							result.Set(k, v)
						}
					}
					continue
				}
			}
			if expanded != nil {
				result.Set(k, expanded)
			}
		}
		return result

	} else if self.Literal != nil {
		return self.Literal

	} else if self.NestedArray != nil {
		result := []interface{}{}
		for _, i := range self.NestedArray {
			result = append(result, i.Expand(args))
		}
		return result

	} else if args != nil {
		value, pres := args[int(self.Id)]
		if !pres {
			return nil
		}

		return value
	}

	return nil
}

func (self *TemplateNode) SetLiteral(key string, literal interface{}) {
	if self.NestedDict == nil {
		self.NestedDict = ordereddict.NewDict() //make(map[string]*TemplateNode)
	}

	// Ignore useless xmlsn attributes.
	if key != "xmlns" {
		self.NestedDict.Set(key, &TemplateNode{Literal: literal})
	}
}

func (self *TemplateNode) SetExpansion(key string, id, type_id uint32) {
	if self.NestedDict == nil {
		self.NestedDict = ordereddict.NewDict() //make(map[string]*TemplateNode)
	}

	self.NestedDict.Set(key, &TemplateNode{Id: id, Type: type_id})
}

func (self *TemplateNode) SetNested(key string, nested *TemplateNode) {
	if self.NestedDict == nil {
		self.NestedDict = ordereddict.NewDict() //make(map[string]*TemplateNode)
	}

	existing_any, pres := self.NestedDict.Get(key)
	if pres {
		existing := existing_any.(*TemplateNode)

		// If there is already a nested value we append it.
		if existing.NestedArray != nil {
			existing.NestedArray = append(existing.NestedArray, nested)
			return
		}

		// Otherwise we convert the existing value to an array
		// and append the new value on it.
		nested = &TemplateNode{
			NestedArray: []*TemplateNode{existing, nested},
		}
	}
	self.NestedDict.Set(key, nested)
}

func NewTemplate(id int) *TemplateNode {
	result := TemplateNode{}
	return &result
}

type ParseContext struct {
	buff   []byte
	offset int

	root *TemplateNode

	// XML Attributes are written to this template.
	stack []*TemplateNode

	// Remember the attribute we are currently parsing.
	current_keys []string

	attribute_mode bool

	// A
	chunk *Chunk

	// A lookup table of templates we already saw in this
	// chunk. Further events in the chunk well reuse the same
	// templates by id.
	knownIDs map[int]*TemplateNode
}

func (self *ParseContext) CurrentKey() string {
	if !self.attribute_mode {
		return ""
	}
	return self.CurrentTemplate().CurrentKey
}

func (self *ParseContext) Offset() int {
	return self.offset
}

func (self *ParseContext) SetOffset(offset int) {
	self.offset = offset
}

func (self *ParseContext) PushTemplate(key string, template *TemplateNode) {
	debug("PushTemplate: %x -> %x\n", len(self.stack), len(self.stack)+1)
	current := self.CurrentTemplate()
	current.SetNested(key, template)
	self.stack = append(self.stack, template)
}

func (self *ParseContext) CurrentTemplate() *TemplateNode {
	if len(self.stack) > 0 {
		return self.stack[len(self.stack)-1]
	}
	return NewTemplate(0)
}

func (self *ParseContext) PopTemplate() {
	if len(self.stack) > 0 {
		debug("PopTemplate: %x -> %x\n", len(self.stack), len(self.stack)-1)
		self.stack = self.stack[:len(self.stack)-1]
	}
}

func NewParseContext(chunk *Chunk) *ParseContext {
	template := NewTemplate(0)
	result := ParseContext{
		root:     template,
		stack:    []*TemplateNode{template},
		chunk:    chunk,
		knownIDs: make(map[int]*TemplateNode),
	}

	return &result
}

func (self *ParseContext) ConsumeUint8() uint8 {
	if len(self.buff) < self.offset+1 {
		return 0
	}
	result := self.buff[self.offset]
	self.offset++
	return result
}

func (self *ParseContext) ConsumeUint16() uint16 {
	if len(self.buff) < self.offset+2 {
		return 0
	}

	result := binary.LittleEndian.Uint16(self.buff[self.offset:])
	self.offset += 2
	return result
}

func (self *ParseContext) ConsumeUint32() uint32 {
	if len(self.buff) < self.offset+4 {
		return 0
	}

	result := binary.LittleEndian.Uint32(self.buff[self.offset:])
	self.offset += 4
	return result
}

func (self *ParseContext) ConsumeUint64() uint64 {
	if len(self.buff) < self.offset+8 {
		return 0
	}

	result := binary.LittleEndian.Uint64(self.buff[self.offset:])
	self.offset += 8
	return result
}

func (self *ParseContext) ConsumeBytes(size int) []byte {
	if self.offset+size > len(self.buff) {
		return make([]byte, size)
	}

	result := self.buff[self.offset : self.offset+size]
	self.offset += size
	return result
}

func (self *ParseContext) SkipBytes(count int) {
	self.offset += count
}

// Make a copy of the context. This new copy can be used to continue
// parsing without disturbing the state of this parser context.
func (self ParseContext) Copy() *ParseContext {
	result := NewParseContext(self.chunk)
	result.buff = self.buff
	result.offset = self.offset
	result.knownIDs = self.knownIDs
	return result
}

func (self *ParseContext) NewTemplate(id int) *TemplateNode {
	self.root = NewTemplate(id)
	self.stack = []*TemplateNode{self.root}

	if id != 0 {
		self.knownIDs[id] = self.root
	}

	return self.root
}

func (self *ParseContext) GetTemplateByID(id int) (*TemplateNode, bool) {
	template, pres := self.knownIDs[id]
	return template, pres
}

func UTF16LEToUTF8(data []byte) []byte {
	if len(data) == 0 || len(data)%2 == 1 {
		return data
	}

	buff := make([]uint16, len(data)/2)
	for i := range buff {
		buff[i] = uint16(data[i*2]) + (uint16(data[i*2+1]) << 8)
	}
	for len(buff) > 0 && buff[len(buff)-1] == 0 {
		buff = buff[:len(buff)-1]
	}

	return []byte(string(utf16.Decode(buff)))
}

func ReadPrefixedUnicodeString(ctx *ParseContext, is_null_terminated bool) string {
	debug("ReadPrefixedUnicodeString Enter: %x\n", ctx.Offset())
	count := int(ctx.ConsumeUint16())
	if is_null_terminated {
		count += 1
	}
	debug("ReadPrefixedUnicodeString count: %d\n", count)
	buffer := ctx.ConsumeBytes(count * 2)
	result := UTF16LEToUTF8(buffer)
	debug("ReadPrefixedUnicodeString exit: %x %s\n", ctx.Offset(), string(result))
	return string(result)
}

func ReadName(ctx *ParseContext) string {
	debug("ReadName Enter: %x\n", ctx.Offset())
	chunkOffset := int(ctx.ConsumeUint32())
	debug("chunkOffset %x ctx offset %x\n", chunkOffset, ctx.Offset())

	// Strings may be interned by reusing the location of the
	// string elsewhere in the chunk.
	if chunkOffset != ctx.Offset() {
		temp_ctx := ctx.Copy()
		temp_ctx.SetOffset(chunkOffset)

		temp_ctx.SkipBytes(4 + 2)
		return ReadPrefixedUnicodeString(temp_ctx, true)
	}

	ctx.SkipBytes(4 + 2)
	return ReadPrefixedUnicodeString(ctx, true)
}

// This is called when we open a new XML Tag. e.g. "<EventData".
func ParseOpenStartElement(ctx *ParseContext, has_attr bool) bool {
	debug("ParseOpenStartElement Enter: %x\n", ctx.Offset())
	/*
		dependencyID := ctx.ConsumeUint16()
		elementLength := ctx.ConsumeUint32()
	*/
	ctx.SkipBytes(2 + 4)
	nameBuffer := ReadName(ctx)

	attributeListLength := uint32(0)
	if has_attr {
		attributeListLength = ctx.ConsumeUint32()
	}

	debug("Start element %v with %v attributes\n", nameBuffer, attributeListLength)

	debug("ParseOpenStartElement Exit: %x\n", ctx.Offset())

	new_template := NewTemplate(0)
	ctx.PushTemplate(nameBuffer, new_template)

	return true
}

// Represents a close of the start element ('>' in <Element>)
func ParseCloseStartElement(ctx *ParseContext) bool {
	debug("ParseCloseStartElement %x\n", ctx.Offset())
	ctx.attribute_mode = false
	ctx.CurrentTemplate().CurrentKey = ""

	return true
}

// Represents a closing element (i.e. </Element>)
func ParseCloseElement(ctx *ParseContext) bool {
	debug("ParseCloseElement %x\n", ctx.Offset())
	ctx.PopTemplate()
	return true
}

func ParseValueText(ctx *ParseContext) bool {
	debug("ParseValueText %x\n", ctx.Offset())
	string_type := ctx.ConsumeUint8()
	string_value := ReadPrefixedUnicodeString(ctx, false)

	debug("ParseValueText Value is %v (type %v)\n",
		string_value, string_type)

	debug("Current Key %v\n", ctx.CurrentKey())

	key := ctx.CurrentKey()
	ctx.CurrentTemplate().SetLiteral(key, string_value)
	ctx.attribute_mode = false

	return true
}

func ParseAttributes(ctx *ParseContext) bool {
	debug("ParseAttributes %x\n", ctx.Offset())
	attribute := ReadName(ctx)

	debug("Attribute is %v\n", attribute)
	ctx.CurrentTemplate().CurrentKey = attribute
	ctx.attribute_mode = true

	return true
}

func ParseTemplateInstance(ctx *ParseContext) bool {
	debug("ParseTemplateInstance Enter %x\n", ctx.Offset())
	if ctx.ConsumeUint8() != 0x01 {
		return false
	}

	short_id := int(ctx.ConsumeUint32())
	if short_id == 0 {
		return false
	}

	/*
		tempResLen := ctx.ConsumeUint32()
	*/
	ctx.SkipBytes(4)

	numArguments := ctx.ConsumeUint32()

	debug("template id %x\n", short_id)

	template, pres := ctx.GetTemplateByID(short_id)
	if !pres {
		// longID := ctx.ConsumeBytes(16)
		ctx.SkipBytes(16)
		templateBodyLen := int(ctx.ConsumeUint32())

		tmp_ctx := ctx.Copy()
		template = tmp_ctx.NewTemplate(short_id)
		ParseBinXML(tmp_ctx)

		ctx.SkipBytes(templateBodyLen)
		numArguments = ctx.ConsumeUint32()
	}

	debug("ParseTemplateInstance Parse %x args @ %x\n", numArguments, ctx.Offset())

	type arg_detail struct {
		argLen  int
		argType uint16
	}
	args := []arg_detail{}
	for i := 0; i < int(numArguments); i++ {
		argLen := ctx.ConsumeUint16()
		argType := ctx.ConsumeUint16()
		args = append(args, arg_detail{int(argLen), argType})
	}

	arg_values := make(map[int]interface{})

	for idx, arg := range args {
		switch arg.argType {
		case 0x00:
			ctx.SkipBytes(arg.argLen)

		case 0x01: // String
			arg_values[idx] = string(UTF16LEToUTF8(ctx.ConsumeBytes(arg.argLen)))
		case 0x04: // uint8_t
			arg_values[idx] = ctx.ConsumeUint8()
		case 0x06: // uint16_t
			arg_values[idx] = ctx.ConsumeUint16()
		case 0x08: // uint32_t
			arg_values[idx] = ctx.ConsumeUint32()
		case 0x0A: // uint64_t
			arg_values[idx] = ctx.ConsumeUint64()
		case 0x0d: // bool
			value := false
			switch arg.argLen {
			case 8:
				value = ctx.ConsumeUint64() > 0
			case 4:
				value = ctx.ConsumeUint32() > 0
			case 2:
				value = ctx.ConsumeUint16() > 0
			case 1:
				value = ctx.ConsumeUint8() > 0
			}
			arg_values[idx] = value
		case 0xe: // binary
			arg_values[idx] = ctx.ConsumeBytes(arg.argLen)
		case 0x0f: // GUID
			guid := EvtxGUID{}
			readStructFromFile(
				bytes.NewReader(ctx.ConsumeBytes(arg.argLen)), 0,
				&guid)
			arg_values[idx] = guid.ToString()

			// We can always format this into hex if we
			// need to. It is better to keep it as an int.
		case 0x14: // HexInt32
			// arg_values[idx] = fmt.Sprintf("%x", ctx.ConsumeUint32())
			arg_values[idx] = ctx.ConsumeUint32()
		case 0x15: // HexInt64
			// arg_values[idx] = fmt.Sprintf("%x", ctx.ConsumeUint64())
			arg_values[idx] = ctx.ConsumeUint64()

		case 0x11: // FileTime - format as seconds since epoch.
			arg_values[idx] = filetimeToUnixtime(ctx.ConsumeUint64())

		case 0x13: // SID
			str := "S"
			str += fmt.Sprintf("-%d", ctx.ConsumeUint8())

			ctx.ConsumeUint8()
			v_q := uint64(0)
			for _, b := range ctx.ConsumeBytes(6) {
				v_q = (v_q << 8) | uint64(b)
			}

			str += fmt.Sprintf("-%d", v_q)
			for idx := 0; idx < arg.argLen-8; idx += 4 {
				str += fmt.Sprintf("-%d", ctx.ConsumeUint32())
			}
			arg_values[idx] = str
		case 0x21: // BinXml
			new_ctx := ctx.Copy()
			ParseBinXML(new_ctx)
			ctx.SkipBytes(arg.argLen)

			arg_values[idx] = new_ctx.CurrentTemplate().Expand(nil)

		case 0x27, 0x28:
			arg_values[idx] = string(ctx.ConsumeBytes(arg.argLen))

		case 0x81: // List of UTF16 String
			arg_values[idx] = strings.Split(
				string(UTF16LEToUTF8(ctx.ConsumeBytes(arg.argLen))),
				"\x00")

		default:
			unknown := ctx.ConsumeBytes(arg.argLen)
			debug("I dont know how to handle %v (%v)\n", arg, unknown)
			arg_values[idx] = strings.TrimRight(string(unknown), "\x00")
		}

		debug("%v Arg type %x len %x - %v\n",
			idx, arg.argType, arg.argLen, arg_values[idx])
	}

	debug("ParseTemplateInstance Exit %x\n", ctx.offset)
	expanded := template.Expand(arg_values)

	NormalizeEventData(expanded)

	ctx.CurrentTemplate().SetLiteral(ctx.CurrentKey(), expanded)

	return true
}

func ParseOptionalSubstitution(ctx *ParseContext) bool {
	debug("ParseOptionalSubstitution Enter %x\n", ctx.Offset())
	substitutionID := ctx.ConsumeUint16()
	valueType := ctx.ConsumeUint8()
	if valueType == 0 {
		valueType = ctx.ConsumeUint8()
	}

	debug("CurrentKey %v\n", ctx.CurrentKey())
	debug("ParseOptionalSubstitution Exit @%x  %x (%x)\n",
		ctx.Offset(), substitutionID, valueType)

	key := ctx.CurrentKey()
	ctx.CurrentTemplate().SetExpansion(key,
		uint32(substitutionID), uint32(valueType))

	return true
}

func ParseBinXML(ctx *ParseContext) {
	debug("ParseBinXML\n")
	keep_going := true

	for keep_going {
		tag := ctx.ConsumeUint8()
		debug("Tag %x @ %x\n", tag, ctx.Offset())
		switch tag {
		case 0x00 /* EOF */ :
			keep_going = false

		case 0x01 /* OpenStartElementToken */ :
			keep_going = ParseOpenStartElement(ctx, false)
		case 0x41:
			keep_going = ParseOpenStartElement(ctx, true)
		case 0x02: /* CloseStartElementToken */
			keep_going = ParseCloseStartElement(ctx)
		case 0x03 /*  CloseEmptyElementToken */, 0x04: /*  CloseElementToken */
			keep_going = ParseCloseElement(ctx)
		case 0x05 /*  ValueTextToken */, 0x45:
			keep_going = ParseValueText(ctx)
		case 0x06 /*  AttributeToken */, 0x46:
			keep_going = ParseAttributes(ctx)
		case 0x07 /* CDATASectionToken */, 0x47:
		case 0x08 /* CharRefToken */, 0x48:
		case 0x09 /*  EntityRefToken */, 0x49:
		case 0x0A /*  PITargetToken */ :
		case 0x0B /*  PIDataToken */ :
		case 0x0C /*  TemplateInstanceToken */ :
			keep_going = ParseTemplateInstance(ctx)
		case 0x0D /*  NormalSubstitutionToken */, 0x0E: /*  OptionalSubstitutionToken */
			keep_going = ParseOptionalSubstitution(ctx)

		case 0x0F /*  FragmentHeaderToken */ :
			ctx.SkipBytes(3)

		default:
			keep_going = false
		}
	}
}

func is_supported(minor, major uint16) bool {
	switch major {
	case 3:
		switch minor {
		case 0, 1, 2:
			return true
		}
	}
	return false
}

// Get all the chunks in the file.
func GetChunks(fd io.ReadSeeker) ([]*Chunk, error) {
	result := []*Chunk{}
	header := EVTXHeader{}
	err := readStructFromFile(fd, 0, &header)
	if err != nil {
		return nil, err
	}

	if string(header.Magic[:]) != EVTX_HEADER_MAGIC {
		return nil, errors.New("File is not an EVTX file (wrong magic).")
	}

	if !is_supported(header.MinorVersion, header.MajorVersion) {
		return nil, errors.New("Unsupported EVTX version.")
	}

	for offset := int64(header.HeaderBlockSize); true; offset += EVTX_CHUNK_SIZE {
		chunk, err := NewChunk(fd, offset)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			}
			continue
		}

		if string(chunk.Header.Magic[:]) != EVTX_CHUNK_HEADER_MAGIC ||
			chunk.Header.LastEventRecID == 0xffffffffffffffff {
			continue
		}
		result = append(result, chunk)
	}

	return result, nil
}

func ParseFile(fd io.ReadSeeker) (*ordereddict.Dict, error) {
	header := EVTXHeader{}
	err := readStructFromFile(fd, 0, &header)
	if err != nil {
		return nil, err
	}

	if string(header.Magic[:]) != EVTX_HEADER_MAGIC {
		return nil, errors.New("File is not an EVTX file (wrong magic).")
	}

	if header.MinorVersion != 1 || header.MajorVersion != 3 {
		return nil, errors.New("Unsupported EVTX version.")
	}

	offset := int64(header.HeaderBlockSize)
	for {
		chunk, err := NewChunk(fd, offset)
		if err != nil {
			return nil, err
		}

		if string(chunk.Header.Magic[:]) == EVTX_CHUNK_HEADER_MAGIC {
			records, err := chunk.Parse(0)
			if err != nil {
				return nil, err
			}

			for _, i := range records {
				fmt.Println(i)

			}
		}
		offset += EVTX_CHUNK_SIZE
	}

	return nil, nil
}

func readStructFromFile(fd io.ReadSeeker, offset int64, obj interface{}) error {
	_, err := fd.Seek(offset, os.SEEK_SET)
	if err != nil {
		return errors.Wrap(err, "Seek")
	}

	err = binary.Read(fd, binary.LittleEndian, obj)
	if err != nil {
		return errors.Wrap(err, "Read")
	}

	return nil
}

func filetimeToUnixtime(ft uint64) float64 {
	return (float64(ft) - 11644473600000*10000) / 10000000
}
