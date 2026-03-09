package models

import "time"

type PacketType string

const (
	PacketTypeLogin     PacketType = "login"
	PacketTypeHeartbeat PacketType = "heartbeat"
	PacketTypeLocation  PacketType = "location"
	PacketTypeAlarm     PacketType = "alarm"
	PacketTypeCommand   PacketType = "command"
	PacketTypeUnknown   PacketType = "unknown"
)

type Device struct {
	ID         int64     `json:"id"`
	IMEI       string    `json:"imei"`
	Protocol   string    `json:"protocol"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Telemetry struct {
	ID           int64      `json:"id,omitempty"`
	DeviceIMEI   string     `json:"imei"`
	Protocol     string     `json:"protocol"`
	PacketType   PacketType `json:"packetType"`
	Serial       string     `json:"serial,omitempty"`
	EventIOID    *int       `json:"eventIoId,omitempty"`
	GPSValid     *bool      `json:"gpsValid,omitempty"`
	Latitude     *float64   `json:"latitude,omitempty"`
	Longitude    *float64   `json:"longitude,omitempty"`
	SpeedKPH     *float64   `json:"speedKph,omitempty"`
	Course       *int       `json:"course,omitempty"`
	Satellites   *int       `json:"satellites,omitempty"`
	MCC          *int       `json:"mcc,omitempty"`
	MNC          *int       `json:"mnc,omitempty"`
	LAC          *int       `json:"lac,omitempty"`
	CellID       *int64     `json:"cellId,omitempty"`
	BatteryLevel *int       `json:"batteryLevel,omitempty"`
	SignalLevel  *int       `json:"signalLevel,omitempty"`
	AlarmCode    string     `json:"alarmCode,omitempty"`
	RawPayload   string     `json:"rawPayload,omitempty"`
	DeviceTime   *time.Time `json:"deviceTime,omitempty"`
	ReceivedAt   time.Time  `json:"receivedAt"`
}

type CommandStatus string

const (
	CommandStatusQueued  CommandStatus = "queued"
	CommandStatusSent    CommandStatus = "sent"
	CommandStatusFailed  CommandStatus = "failed"
	CommandStatusAcked   CommandStatus = "acked"
	CommandStatusUnknown CommandStatus = "unknown"
)

type CommandOutbox struct {
	ID           int64         `json:"id"`
	IMEI         string        `json:"imei"`
	Protocol     string        `json:"protocol"`
	CommandText  string        `json:"commandText"`
	Status       CommandStatus `json:"status"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	SentAt       *time.Time    `json:"sentAt,omitempty"`
	AckedAt      *time.Time    `json:"ackedAt,omitempty"`
}
