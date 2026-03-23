package tunnel

import (
	"encoding/binary"
	"errors"
)

const (
	MsgNewStream   byte = 0x01
	MsgData        byte = 0x02
	MsgCloseStream byte = 0x03
)

const headerSize = 5

type Message struct {
	Type     byte
	StreamID uint32
	Payload  []byte
}

func encodeMessage(msg *Message) []byte {
	buf := make([]byte, headerSize+len(msg.Payload))
	buf[0] = msg.Type
	binary.BigEndian.PutUint32(buf[1:5], msg.StreamID)
	copy(buf[5:], msg.Payload)
	return buf
}

func decodeMessage(data []byte) (*Message, error) {
	if len(data) < headerSize {
		return nil, errors.New("message too short")
	}
	return &Message{
		Type:     data[0],
		StreamID: binary.BigEndian.Uint32(data[1:5]),
		Payload:  data[5:],
	}, nil
}
