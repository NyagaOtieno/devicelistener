package command

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gps-listener-backend/internal/protocol/common"
	"gps-listener-backend/internal/protocol/uniguard"
)

func Build(protocol, imei, serial, commandText string) ([]byte, error) {
	switch strings.ToLower(protocol) {
	case "gt06":
		return BuildGT06Command(commandText), nil
	case "uniguard":
		if serial == "" {
			serial = fmt.Sprintf("%04x", time.Now().UnixNano()&0xffff)
		}
		return []byte(uniguard.BuildFrame("S168", imei, serial, commandText)), nil
	case "teltonika-fmb920", "teltonika":
		return BuildTeltonikaCodec12(commandText), nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", protocol)
	}
}

func BuildGT06Command(commandText string) []byte {
	commandText = strings.TrimSpace(commandText)
	if !strings.HasSuffix(commandText, "#") {
		commandText += "#"
	}
	serverFlag := make([]byte, 4)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(serverFlag)
	serial := uint16(time.Now().UnixNano() & 0xffff)
	cmdBytes := []byte(commandText)
	body := []byte{byte(1 + 4 + len(cmdBytes) + 2 + 2), 0x80, byte(4 + len(cmdBytes))}
	body = append(body, serverFlag...)
	body = append(body, cmdBytes...)
	body = append(body, byte(serial>>8), byte(serial))
	crc := common.CRC16ITU(body)
	frame := []byte{0x78, 0x78}
	frame = append(frame, body...)
	frame = append(frame, byte(crc>>8), byte(crc), 0x0D, 0x0A)
	return frame
}

func BuildTeltonikaCodec12(commandText string) []byte {
	payload := []byte(strings.TrimSpace(commandText))
	data := []byte{0x0C, 0x01, 0x05}
	sz := make([]byte, 4)
	binary.BigEndian.PutUint32(sz, uint32(len(payload)))
	data = append(data, sz...)
	data = append(data, payload...)
	data = append(data, 0x01)
	frame := make([]byte, 8)
	binary.BigEndian.PutUint32(frame[0:4], 0)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(data)))
	frame = append(frame, data...)
	crc := common.CRC16IBM(data)
	crc4 := make([]byte, 4)
	binary.BigEndian.PutUint32(crc4, uint32(crc))
	frame = append(frame, crc4...)
	return frame
}

func BytesToHexUpper(b []byte) string { return strings.ToUpper(hex.EncodeToString(b)) }
