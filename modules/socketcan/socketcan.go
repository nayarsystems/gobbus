package socketcan

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

type FrameType uint

const (
	SFF FrameType = iota // SFF frame format
	EFF                  // EFF extended frame format
	RTR                  // RTR frame format
	ERR                  // ERR frame format
)

type frame struct {
	id      uint32
	dlc     uint8
	padding [3]byte
	data    [8]byte
}

// NewCanFrame creates a frame ready to be sent to the socketcan module
func NewCanFrame(id uint32, ty FrameType, data []byte) ([]byte, error) {
	frame := &frame{id: id, dlc: uint8(len(data))}

	if frame.dlc > 8 {
		return nil, fmt.Errorf("Data excedes 8 bytes")
	} else if frame.dlc > 0 {
		copy(frame.data[:frame.dlc], data)
	}

	switch ty {
	case SFF:
		if id > unix.CAN_SFF_MASK {
			return nil, fmt.Errorf("SFF frame type IDs only support up to 11 bits (maximum value %d)", unix.CAN_SFF_MASK)
		}
		frame.id &= unix.CAN_SFF_MASK
	case EFF:
		if id > unix.CAN_EFF_MASK {
			return nil, fmt.Errorf("EFF frame type IDs only support up to 29 bits (maximum value %d)", unix.CAN_EFF_MASK)
		}
		frame.id &= unix.CAN_EFF_MASK
		frame.id |= unix.CAN_EFF_FLAG
	case RTR:
		if id > unix.CAN_EFF_MASK {
			return nil, fmt.Errorf("RTR frame type IDs only support up to 29 bits (maximum value %d)", unix.CAN_EFF_MASK)
		}
		frame.id &= unix.CAN_EFF_MASK
		frame.id |= unix.CAN_RTR_FLAG
	case ERR:
		if id > unix.CAN_EFF_MASK {
			return nil, fmt.Errorf("ERR frame type IDs only support up to 29 bits (maximum value %d)", unix.CAN_EFF_MASK)
		}
		frame.id &= unix.CAN_ERR_MASK
		frame.id |= unix.CAN_ERR_FLAG
	default:
		return nil, fmt.Errorf("Invalid frame type")
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, frame)
	return buf.Bytes(), nil
}
