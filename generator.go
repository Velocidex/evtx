package evtx

import (
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
)

type GeneratedEvent struct {
	Event map[string]interface{}
	Err   error
}

func GenerateEvents(fd io.ReadSeeker) (chan GeneratedEvent, func(), error) {
	header := EVTXHeader{}
	err := readStructFromFile(fd, 0, &header)
	if err != nil {
		return nil, nil, err
	}

	if string(header.Magic[:]) != EVTX_HEADER_MAGIC {
		return nil, nil, errors.New("file is not an EVTX file (wrong magic)")
	}

	if !is_supported(header.MinorVersion, header.MajorVersion) {
		return nil, nil, errors.New("unsupported EVTX version")
	}

	chClose := make(chan struct{})
	chEvents := make(chan GeneratedEvent)

	genEvent := func(e GeneratedEvent) bool {
		select {
		case chEvents <- e:
			return true
		case <-chClose:
			return false
		}
	}

	go func() {
		defer close(chEvents)
		offset := int64(header.HeaderBlockSize)
		for {
			chunk, err := NewChunk(fd, offset)
			if err != nil {
				genEvent(GeneratedEvent{Event: nil, Err: err})
				return
			}

			if string(chunk.Header.Magic[:]) == EVTX_CHUNK_HEADER_MAGIC {
				records, err := chunk.Parse(0)
				if err != nil {
					return
				}
				for _, i := range records {
					event_map, ok := i.Event.(*ordereddict.Dict)
					if !ok {
						continue
					}
					m := event_map.ToDict()
					if m == nil {
						continue
					}
					if !genEvent(GeneratedEvent{Event: *m, Err: err}) {
						return
					}
				}
			}
			offset += EVTX_CHUNK_SIZE
		}
	}()

	return chEvents, func() { chClose <- struct{}{} }, nil
}
