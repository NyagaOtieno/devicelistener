package gt06

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"gps-listener-backend/internal/models"
	"gps-listener-backend/internal/protocol/common"
)

const (
	ProtoLogin    byte = 0x01
	ProtoLocation byte = 0x12
	ProtoStatus   byte = 0x13
	ProtoString   byte = 0x15
	ProtoAlarm    byte = 0x16
	ProtoCommand  byte = 0x80
)

type Packet struct {
	ProtocolNumber byte
	Serial         uint16
	IMEI           string
	Telemetry      *models.Telemetry
	RawHex         string
}

func IsFrame(data []byte) bool {
	return len(data) >= 5 && data[0] == 0x78 && data[1] == 0x78
}

func Parse(frame []byte) (*Packet, error) {
	if len(frame) < 10 {
		return nil, fmt.Errorf("frame too short")
	}
	if frame[0] != 0x78 || frame[1] != 0x78 {
		return nil, fmt.Errorf("invalid start bits")
	}
	if frame[len(frame)-2] != 0x0D || frame[len(frame)-1] != 0x0A {
		return nil, fmt.Errorf("invalid stop bits")
	}

	length := int(frame[2])
	expected := length + 5
	if len(frame) != expected {
		return nil, fmt.Errorf("invalid frame length: got %d want %d", len(frame), expected)
	}

	crcStart := 2
	crcEnd := len(frame) - 4
	wantCRC := binary.BigEndian.Uint16(frame[len(frame)-4 : len(frame)-2])
	gotCRC := common.CRC16ITU(frame[crcStart:crcEnd])
	if wantCRC != gotCRC {
		return nil, fmt.Errorf("crc mismatch: got %04X want %04X", gotCRC, wantCRC)
	}

	proto := frame[3]
	serial := binary.BigEndian.Uint16(frame[len(frame)-6 : len(frame)-4])
	pkt := &Packet{
		ProtocolNumber: proto,
		Serial:         serial,
		RawHex:         hex.EncodeToString(frame),
	}

	switch proto {
	case ProtoLogin:
		imei, err := decodeIMEI(frame[4:12])
		if err != nil {
			return nil, err
		}
		pkt.IMEI = imei
		pkt.Telemetry = &models.Telemetry{
			DeviceIMEI: imei,
			Protocol:   "gt06",
			PacketType: models.PacketTypeLogin,
			Serial:     fmt.Sprintf("%04X", serial),
			RawPayload: pkt.RawHex,
			ReceivedAt: time.Now().UTC(),
		}
	case ProtoLocation:
		t, err := parseLocation(frame, serial)
		if err != nil {
			return nil, err
		}
		pkt.Telemetry = t
		pkt.IMEI = ""
	case ProtoStatus:
		t := parseStatus(frame, serial)
		pkt.Telemetry = t
	case ProtoAlarm:
		t, err := parseAlarm(frame, serial)
		if err != nil {
			return nil, err
		}
		pkt.Telemetry = t
	case ProtoString:
		t := parseStringResponse(frame, serial)
		pkt.Telemetry = t
	default:
		pkt.Telemetry = &models.Telemetry{Protocol: "gt06", PacketType: models.PacketTypeUnknown, Serial: fmt.Sprintf("%04X", serial), RawPayload: pkt.RawHex, ReceivedAt: time.Now().UTC()}
	}

	return pkt, nil
}

func BuildAck(protocol byte, serial uint16) []byte {
	body := []byte{0x05, protocol, byte(serial >> 8), byte(serial)}
	crc := common.CRC16ITU(body)
	frame := []byte{0x78, 0x78}
	frame = append(frame, body...)
	frame = append(frame, byte(crc>>8), byte(crc))
	frame = append(frame, 0x0D, 0x0A)
	return frame
}

func decodeIMEI(b []byte) (string, error) {
	if len(b) != 8 {
		return "", fmt.Errorf("invalid imei length")
	}
	encoded := hex.EncodeToString(b)
	if len(encoded) < 16 {
		return "", fmt.Errorf("invalid imei encoding")
	}
	return encoded[1:], nil
}

func parseGT06Time(b []byte) (*time.Time, error) {
	if len(b) != 6 {
		return nil, fmt.Errorf("invalid datetime length")
	}
	t := time.Date(2000+int(b[0]), time.Month(b[1]), int(b[2]), int(b[3]), int(b[4]), int(b[5]), 0, time.UTC)
	return &t, nil
}

func parseLocation(frame []byte, serial uint16) (*models.Telemetry, error) {
	if len(frame) < 37 {
		return nil, fmt.Errorf("location frame too short")
	}
	devTime, err := parseGT06Time(frame[4:10])
	if err != nil {
		return nil, err
	}
	gpsLenSat := frame[10]
	satellites := int(gpsLenSat & 0x0F)
	latRaw := binary.BigEndian.Uint32(frame[11:15])
	lonRaw := binary.BigEndian.Uint32(frame[15:19])
	speed := float64(frame[19])
	courseStatus := binary.BigEndian.Uint16(frame[20:22])
	mcc := int(binary.BigEndian.Uint16(frame[22:24]))
	mnc := int(frame[24])
	lac := int(binary.BigEndian.Uint16(frame[25:27]))
	cellID := int64(uint32(frame[27])<<16 | uint32(frame[28])<<8 | uint32(frame[29]))
	gpsValid := courseStatus&(1<<12) != 0
	west := courseStatus&(1<<11) != 0
	south := courseStatus&(1<<10) != 0
	course := int(courseStatus & 0x03FF)
	lat := float64(latRaw) / 30000.0 / 60.0
	lon := float64(lonRaw) / 30000.0 / 60.0
	if south {
		lat = -lat
	}
	if west {
		lon = -lon
	}
	return &models.Telemetry{
		Protocol:   "gt06",
		PacketType: models.PacketTypeLocation,
		Serial:     fmt.Sprintf("%04X", serial),
		GPSValid:   boolPtr(gpsValid),
		Latitude:   &lat,
		Longitude:  &lon,
		SpeedKPH:   &speed,
		Course:     &course,
		Satellites: &satellites,
		MCC:        &mcc,
		MNC:        &mnc,
		LAC:        &lac,
		CellID:     &cellID,
		RawPayload: hex.EncodeToString(frame),
		DeviceTime: devTime,
		ReceivedAt: time.Now().UTC(),
	}, nil
}

func parseStatus(frame []byte, serial uint16) *models.Telemetry {
	termInfo := int(frame[4])
	voltage := int(frame[5])
	signal := int(frame[6])
	alarmLang := binary.BigEndian.Uint16(frame[7:9])
	alarm := fmt.Sprintf("%04X", alarmLang)
	return &models.Telemetry{
		Protocol:     "gt06",
		PacketType:   models.PacketTypeHeartbeat,
		Serial:       fmt.Sprintf("%04X", serial),
		BatteryLevel: &voltage,
		SignalLevel:  &signal,
		AlarmCode:    alarm,
		RawPayload:   hex.EncodeToString(frame),
		ReceivedAt:   time.Now().UTC(),
		Course:       &termInfo,
	}
}

func parseAlarm(frame []byte, serial uint16) (*models.Telemetry, error) {
	if len(frame) < 42 {
		return nil, fmt.Errorf("alarm frame too short")
	}
	devTime, err := parseGT06Time(frame[4:10])
	if err != nil {
		return nil, err
	}
	gpsLenSat := frame[10]
	satellites := int(gpsLenSat & 0x0F)
	latRaw := binary.BigEndian.Uint32(frame[11:15])
	lonRaw := binary.BigEndian.Uint32(frame[15:19])
	speed := float64(frame[19])
	courseStatus := binary.BigEndian.Uint16(frame[20:22])
	mcc := int(binary.BigEndian.Uint16(frame[23:25]))
	mnc := int(frame[25])
	lac := int(binary.BigEndian.Uint16(frame[26:28]))
	cellID := int64(uint32(frame[28])<<16 | uint32(frame[29])<<8 | uint32(frame[30]))
	termInfo := int(frame[31])
	voltage := int(frame[32])
	signal := int(frame[33])
	alarmLang := binary.BigEndian.Uint16(frame[34:36])
	gpsValid := courseStatus&(1<<12) != 0
	west := courseStatus&(1<<11) != 0
	south := courseStatus&(1<<10) != 0
	course := int(courseStatus & 0x03FF)
	lat := float64(latRaw) / 30000.0 / 60.0
	lon := float64(lonRaw) / 30000.0 / 60.0
	if south {
		lat = -lat
	}
	if west {
		lon = -lon
	}
	alarm := fmt.Sprintf("term=%02X alarm=%04X", termInfo, alarmLang)
	return &models.Telemetry{
		Protocol:     "gt06",
		PacketType:   models.PacketTypeAlarm,
		Serial:       fmt.Sprintf("%04X", serial),
		GPSValid:     boolPtr(gpsValid),
		Latitude:     &lat,
		Longitude:    &lon,
		SpeedKPH:     &speed,
		Course:       &course,
		Satellites:   &satellites,
		MCC:          &mcc,
		MNC:          &mnc,
		LAC:          &lac,
		CellID:       &cellID,
		BatteryLevel: &voltage,
		SignalLevel:  &signal,
		AlarmCode:    alarm,
		RawPayload:   hex.EncodeToString(frame),
		DeviceTime:   devTime,
		ReceivedAt:   time.Now().UTC(),
	}, nil
}

func boolPtr(v bool) *bool { return &v }

func parseStringResponse(frame []byte, serial uint16) *models.Telemetry {
	if len(frame) < 16 {
		return &models.Telemetry{Protocol: "gt06", PacketType: models.PacketTypeCommand, Serial: fmt.Sprintf("%04X", serial), RawPayload: hex.EncodeToString(frame), ReceivedAt: time.Now().UTC()}
	}
	cmdLen := int(frame[4])
	start := 9
	end := start + (cmdLen - 4)
	if end > len(frame)-6 {
		end = len(frame) - 6
	}
	resp := string(frame[start:end])
	return &models.Telemetry{Protocol: "gt06", PacketType: models.PacketTypeCommand, Serial: fmt.Sprintf("%04X", serial), AlarmCode: resp, RawPayload: hex.EncodeToString(frame), ReceivedAt: time.Now().UTC()}
}
