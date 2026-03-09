package uniguard

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gps-listener-backend/internal/models"
)

type Frame struct {
	ID        string
	IMEI      string
	Serial    string
	LengthHex string
	Content   string
	Keywords  map[string]string
	Telemetry *models.Telemetry
	Raw       string
}

func IsFrame(s string) bool {
	return strings.Contains(s, "#") && strings.HasSuffix(strings.TrimSpace(s), "$")
}

func Parse(raw string) (*Frame, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "#")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid uniguard frame")
	}
	last := parts[len(parts)-1]
	if !strings.HasSuffix(last, "$") {
		return nil, fmt.Errorf("missing terminator")
	}
	content := strings.TrimSuffix(strings.Join(parts[4:], "#"), "$")
	f := &Frame{
		ID:        strings.TrimSpace(parts[0]),
		IMEI:      strings.TrimSpace(parts[1]),
		Serial:    strings.TrimSpace(parts[2]),
		LengthHex: strings.TrimSpace(parts[3]),
		Content:   strings.TrimSpace(content),
		Keywords:  parseKeywords(content),
		Raw:       raw,
	}
	if err := validateLength(f.LengthHex, f.Content); err != nil {
		return nil, err
	}
	f.Telemetry = buildTelemetry(f)
	return f, nil
}

func parseKeywords(content string) map[string]string {
	out := map[string]string{}
	chunks := strings.Split(content, ";")
	for _, ch := range chunks {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		if strings.Contains(ch, ":") {
			parts := strings.SplitN(ch, ":", 2)
			out[strings.ToUpper(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
			continue
		}
		csv := strings.Split(ch, ",")
		key := strings.ToUpper(strings.TrimSpace(csv[0]))
		value := ""
		if len(csv) > 1 {
			value = strings.TrimSpace(strings.Join(csv[1:], ","))
		}
		out[key] = value
	}
	return out
}

func buildTelemetry(f *Frame) *models.Telemetry {
	now := time.Now().UTC()
	t := &models.Telemetry{
		DeviceIMEI: f.IMEI,
		Protocol:   "uniguard",
		Serial:     strings.ToUpper(f.Serial),
		RawPayload: f.Raw,
		ReceivedAt: now,
		PacketType: models.PacketTypeUnknown,
	}
	if _, ok := f.Keywords["LOCA"]; ok {
		t.PacketType = models.PacketTypeLocation
		gps := splitCSV(f.Keywords["GDATA"])
		if len(gps) >= 8 {
			valid := strings.EqualFold(gps[0], "A")
			t.GPSValid = &valid
			if sat, err := strconv.Atoi(strings.TrimSpace(gps[1])); err == nil {
				t.Satellites = &sat
			}
			if ts, err := time.Parse("060102150405", strings.TrimSpace(gps[2])); err == nil {
				ts = ts.UTC()
				t.DeviceTime = &ts
			}
			if lat, err := strconv.ParseFloat(strings.TrimSpace(gps[3]), 64); err == nil {
				t.Latitude = &lat
			}
			if lon, err := strconv.ParseFloat(strings.TrimSpace(gps[4]), 64); err == nil {
				t.Longitude = &lon
			}
			if speed, err := strconv.ParseFloat(strings.TrimSpace(gps[5]), 64); err == nil {
				t.SpeedKPH = &speed
			}
			if course, err := strconv.Atoi(strings.TrimSpace(gps[6])); err == nil {
				t.Course = &course
			}
		}
		cell := splitCSV(f.Keywords["CELL"])
		if len(cell) >= 5 {
			if mcc, err := strconv.ParseInt(strings.TrimSpace(cell[1]), 16, 64); err == nil {
				v := int(mcc)
				t.MCC = &v
			}
			if mnc, err := strconv.ParseInt(strings.TrimSpace(cell[2]), 16, 64); err == nil {
				v := int(mnc)
				t.MNC = &v
			}
			if lac, err := strconv.ParseInt(strings.TrimSpace(cell[3]), 16, 64); err == nil {
				v := int(lac)
				t.LAC = &v
			}
			if cid, err := strconv.ParseInt(strings.TrimSpace(cell[4]), 16, 64); err == nil {
				t.CellID = &cid
			}
		}
		status := splitCSV(f.Keywords["STATUS"])
		if len(status) >= 2 {
			if batt, err := strconv.Atoi(strings.TrimSpace(status[0])); err == nil {
				t.BatteryLevel = &batt
			}
			if sig, err := strconv.Atoi(strings.TrimSpace(status[1])); err == nil {
				t.SignalLevel = &sig
			}
		}
		if alert, ok := f.Keywords["ALERT"]; ok {
			t.AlarmCode = strings.TrimSpace(alert)
		}
	}
	if _, ok := f.Keywords["SYNC"]; ok {
		t.PacketType = models.PacketTypeHeartbeat
		if status, ok := f.Keywords["STATUS"]; ok {
			v := splitCSV(status)
			if len(v) > 0 {
				if n, err := strconv.ParseInt(strings.TrimSpace(v[0]), 16, 64); err == nil {
					vv := int(n)
					t.BatteryLevel = &vv
				}
			}
		}
	}
	if strings.HasPrefix(strings.ToUpper(f.Content), "RET,") {
		t.PacketType = models.PacketTypeCommand
	}
	return t
}

func BuildAckLOCA(id, imei, serial string) string {
	content := "ACK^LOCA"
	return BuildFrame(id, imei, serial, content)
}

func BuildAckSYNC(id, imei, serial string, now time.Time) string {
	content := fmt.Sprintf("ACK^SYNC,%s", now.UTC().Format("20060102150405"))
	return BuildFrame(id, imei, serial, content)
}

func BuildFrame(id, imei, serial, content string) string {
	length := fmt.Sprintf("%04x", len(content))
	return fmt.Sprintf("%s#%s#%s#%s#%s$", id, imei, strings.ToLower(serial), length, content)
}

func ExpectedAuth(imei, secret string) string {
	sum := md5.Sum([]byte(imei + secret))
	return hex.EncodeToString(sum[:])
}

func validateLength(lengthHex, content string) error {
	n, err := strconv.ParseInt(lengthHex, 16, 64)
	if err != nil {
		return fmt.Errorf("invalid length hex: %w", err)
	}
	if int(n) != len(content) {
		return fmt.Errorf("length mismatch: header=%d actual=%d", n, len(content))
	}
	return nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
