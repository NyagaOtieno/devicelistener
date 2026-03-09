package teltonika

import (
	"encoding/binary"
	"fmt"
)

type Codec12Response struct {
	CodecID byte
	Type    byte
	Text    string
}

func ParseCodec12(frame []byte) (*Codec12Response, error) {
	if len(frame) < 17 {
		return nil, fmt.Errorf("codec12 frame too short")
	}
	if binary.BigEndian.Uint32(frame[:4]) != 0 {
		return nil, fmt.Errorf("invalid preamble")
	}
	dataLen := int(binary.BigEndian.Uint32(frame[4:8]))
	if len(frame) != 8+dataLen+4 {
		return nil, fmt.Errorf("invalid codec12 length")
	}
	codec := frame[8]
	if codec != 0x0C {
		return nil, fmt.Errorf("not codec12")
	}
	respType := frame[10]
	txtLen := int(binary.BigEndian.Uint32(frame[11:15]))
	if 15+txtLen > 8+dataLen-1 {
		return nil, fmt.Errorf("invalid codec12 payload length")
	}
	return &Codec12Response{CodecID: codec, Type: respType, Text: string(frame[15 : 15+txtLen])}, nil
}
