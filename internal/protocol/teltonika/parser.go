package teltonika

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"gps-listener-backend/internal/models"
	"gps-listener-backend/internal/protocol/common"
)

const (
	Codec8  byte = 0x08
	Codec8E byte = 0x8E
	Codec16 byte = 0x10
)

type IMEIHandshake struct {
	IMEI string
}

type Packet struct {
	CodecID       byte
	RecordCount   int
	Records       []models.Telemetry
	RawHex        string
	AcceptedCount int
}

func IsHandshake(b []byte) bool {
	return len(b) >= 2 && !(len(b) >= 4 && binary.BigEndian.Uint32(b[:4]) == 0)
}

func ParseHandshake(b []byte) (*IMEIHandshake, error) {
	if len(b) < 3 {
		return nil, fmt.Errorf("handshake too short")
	}

	ln := int(binary.BigEndian.Uint16(b[:2]))
	if len(b) != ln+2 {
		return nil, fmt.Errorf("invalid imei handshake length: got %d want %d", len(b), ln+2)
	}

	imei := string(b[2:])
	if len(imei) < 14 || len(imei) > 17 {
		return nil, fmt.Errorf("invalid imei length %d", len(imei))
	}

	return &IMEIHandshake{IMEI: imei}, nil
}

func ParseAVL(frame []byte, imei string) (*Packet, error) {
	if len(frame) < 12 {
		return nil, fmt.Errorf("avl frame too short")
	}

	if binary.BigEndian.Uint32(frame[:4]) != 0 {
		return nil, fmt.Errorf("invalid preamble")
	}

	dataLen := int(binary.BigEndian.Uint32(frame[4:8]))
	expectedLen := 8 + dataLen + 4
	if len(frame) != expectedLen {
		return nil, fmt.Errorf("invalid avl frame length: got %d want %d", len(frame), expectedLen)
	}

	codec := frame[8]
	if codec != Codec8 && codec != Codec8E && codec != Codec16 {
		return nil, fmt.Errorf("unsupported teltonika codec 0x%02X", codec)
	}

	// payload layout:
	// [codec id][record count 1][records...][record count 2]
	payload := frame[8 : 8+dataLen]

	wantCRC := binary.BigEndian.Uint32(frame[len(frame)-4:])
	gotCRC := uint32(common.CRC16IBM(payload))
	if wantCRC != gotCRC {
		return nil, fmt.Errorf("crc mismatch: got %08X want %08X", gotCRC, wantCRC)
	}

	if len(payload) < 3 {
		return nil, fmt.Errorf("payload too short")
	}

	count1 := int(payload[1])
	pos := 2

	records := make([]models.Telemetry, 0, count1)
	for i := 0; i < count1; i++ {
		rec, next, err := parseRecord(codec, payload, pos, imei)
		if err != nil {
			return nil, fmt.Errorf("parse record %d: %w", i+1, err)
		}
		records = append(records, rec)
		pos = next
	}

	if pos >= len(payload) {
		return nil, fmt.Errorf("missing second record count")
	}

	count2 := int(payload[pos])
	if count1 != count2 {
		return nil, fmt.Errorf("record count mismatch %d != %d", count1, count2)
	}

	return &Packet{
		CodecID:       codec,
		RecordCount:   count1,
		Records:       records,
		RawHex:        hex.EncodeToString(frame),
		AcceptedCount: count1,
	}, nil
}

func BuildIMEIAck(accept bool) []byte {
	if accept {
		return []byte{0x01}
	}
	return []byte{0x00}
}

func BuildAVLAck(count int) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(count))
	return out
}

func parseRecord(codec byte, payload []byte, pos int, imei string) (models.Telemetry, int, error) {
	if pos < 0 || pos >= len(payload) {
		return models.Telemetry{}, pos, fmt.Errorf("record position out of bounds")
	}
	if len(payload[pos:]) < 24 {
		return models.Telemetry{}, pos, fmt.Errorf("record too short")
	}

	tsMillis := int64(binary.BigEndian.Uint64(payload[pos : pos+8]))
	deviceTime := time.UnixMilli(tsMillis).UTC()
	pos += 8

	priority := int(payload[pos])
	pos++

	lonRaw := int32(binary.BigEndian.Uint32(payload[pos : pos+4]))
	pos += 4

	latRaw := int32(binary.BigEndian.Uint32(payload[pos : pos+4]))
	pos += 4

	altitude := int(int16(binary.BigEndian.Uint16(payload[pos : pos+2])))
	pos += 2

	angle := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	sats := int(payload[pos])
	pos++

	speed := float64(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	lat := float64(latRaw) / 10000000.0
	lon := float64(lonRaw) / 10000000.0
	gpsValid := sats > 0

	t := models.Telemetry{
		DeviceIMEI: imei,
		Protocol:   "teltonika-fmb920",
		PacketType: models.PacketTypeLocation,
		Serial:     fmt.Sprintf("prio-%d", priority),
		GPSValid:   boolPtr(gpsValid),
		Latitude:   &lat,
		Longitude:  &lon,
		SpeedKPH:   &speed,
		Course:     &angle,
		Satellites: &sats,
		DeviceTime: &deviceTime,
		ReceivedAt: time.Now().UTC(),
	}

	if altitude != 0 {
		t.AlarmCode = fmt.Sprintf("altitude=%d", altitude)
	}

	switch codec {
	case Codec8:
		next, err := parseCodec8IO(payload, pos, &t)
		return t, next, err
	case Codec8E:
		next, err := parseCodec8EIO(payload, pos, &t)
		return t, next, err
	case Codec16:
		next, err := parseCodec16IO(payload, pos, &t)
		return t, next, err
	default:
		return t, pos, nil
	}
}

func parseCodec8IO(payload []byte, pos int, t *models.Telemetry) (int, error) {
	if len(payload[pos:]) < 2 {
		return pos, fmt.Errorf("codec8 io header too short")
	}

	eventID := int(payload[pos])
	pos++

	total := int(payload[pos])
	pos++
	_ = total

	counts := []int{}
	sizes := []int{1, 2, 4, 8}

	for i := 0; i < 4; i++ {
		if len(payload[pos:]) < 1 {
			return pos, fmt.Errorf("codec8 io count too short")
		}

		counts = append(counts, int(payload[pos]))
		pos++

		for j := 0; j < counts[i]; j++ {
			if len(payload[pos:]) < 1+sizes[i] {
				return pos, fmt.Errorf("codec8 io value too short")
			}

			ioID := int(payload[pos])
			pos++

			value := payload[pos : pos+sizes[i]]
			pos += sizes[i]

			applyIO(ioID, value, t)
		}
	}

	if eventID > 0 {
		t.EventIOID = &eventID
	}

	return pos, nil
}

func parseCodec8EIO(payload []byte, pos int, t *models.Telemetry) (int, error) {
	if len(payload[pos:]) < 4 {
		return pos, fmt.Errorf("codec8e io header too short")
	}

	eventID := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	total := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2
	_ = total

	for _, size := range []int{1, 2, 4, 8} {
		if len(payload[pos:]) < 2 {
			return pos, fmt.Errorf("codec8e io count too short")
		}

		count := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
		pos += 2

		for j := 0; j < count; j++ {
			if len(payload[pos:]) < 2+size {
				return pos, fmt.Errorf("codec8e io value too short")
			}

			ioID := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
			pos += 2

			value := payload[pos : pos+size]
			pos += size

			applyIO(ioID, value, t)
		}
	}

	if len(payload[pos:]) < 2 {
		return pos, fmt.Errorf("codec8e nx count too short")
	}

	nxCount := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	for i := 0; i < nxCount; i++ {
		if len(payload[pos:]) < 4 {
			return pos, fmt.Errorf("codec8e nx header too short")
		}

		ioID := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
		pos += 2

		ln := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
		pos += 2

		if len(payload[pos:]) < ln {
			return pos, fmt.Errorf("codec8e nx value too short")
		}

		value := payload[pos : pos+ln]
		pos += ln

		applyIO(ioID, value, t)
	}

	if eventID > 0 {
		t.EventIOID = &eventID
	}

	return pos, nil
}

func parseCodec16IO(payload []byte, pos int, t *models.Telemetry) (int, error) {
	if len(payload[pos:]) < 5 {
		return pos, fmt.Errorf("codec16 generation/event header too short")
	}

	pos++ // generation type

	eventID := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	total := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2
	_ = total

	for _, size := range []int{1, 2, 4, 8} {
		if len(payload[pos:]) < 2 {
			return pos, fmt.Errorf("codec16 count too short")
		}

		count := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
		pos += 2

		for i := 0; i < count; i++ {
			if len(payload[pos:]) < 2+size {
				return pos, fmt.Errorf("codec16 value too short")
			}

			ioID := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
			pos += 2

			value := payload[pos : pos+size]
			pos += size

			applyIO(ioID, value, t)
		}
	}

	if eventID > 0 {
		t.EventIOID = &eventID
	}

	return pos, nil
}

func applyIO(id int, value []byte, t *models.Telemetry) {
	asUint := func(b []byte) uint64 {
		var v uint64
		for _, x := range b {
			v = (v << 8) | uint64(x)
		}
		return v
	}

	switch id {
	case 66:
		v := int(asUint(value))
		t.BatteryLevel = &v
	case 21:
		v := int(asUint(value))
		t.SignalLevel = &v
	case 239:
		if asUint(value) == 1 {
			t.PacketType = models.PacketTypeHeartbeat
		}
	default:
	}
}

func boolPtr(v bool) *bool { return &v }