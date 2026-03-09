package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gps-listener-backend/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{Pool: pool}, nil
}
func (s *Store) Close() {
	if s != nil && s.Pool != nil {
		s.Pool.Close()
	}
}

func (s *Store) UpsertDevice(ctx context.Context, imei, protocol string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO devices (imei, protocol, last_seen_at) VALUES ($1,$2,NOW()) ON CONFLICT (imei) DO UPDATE SET protocol=EXCLUDED.protocol,last_seen_at=NOW(),updated_at=NOW()`, imei, protocol)
	return err
}
func (s *Store) InsertRawPacket(ctx context.Context, imei, protocol, direction string, payload []byte) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO raw_packets (imei, protocol, direction, payload_hex, received_at) VALUES ($1,$2,$3,encode($4,'hex'),NOW())`, nullIfEmpty(imei), protocol, direction, payload)
	return err
}
func (s *Store) InsertTelemetry(ctx context.Context, t models.Telemetry) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO telemetry (imei, protocol, packet_type, serial, event_io_id, gps_valid, latitude, longitude, speed_kph, course, satellites, mcc, mnc, lac, cell_id, battery_level, signal_level, alarm_code, raw_payload, device_time, received_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, nullIfEmpty(t.DeviceIMEI), t.Protocol, string(t.PacketType), nullIfEmpty(t.Serial), t.EventIOID, t.GPSValid, t.Latitude, t.Longitude, t.SpeedKPH, t.Course, t.Satellites, t.MCC, t.MNC, t.LAC, t.CellID, t.BatteryLevel, t.SignalLevel, nullIfEmpty(t.AlarmCode), t.RawPayload, t.DeviceTime, nonZeroTime(t.ReceivedAt))
	return err
}
func (s *Store) ListDevices(ctx context.Context) ([]models.Device, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, imei, protocol, COALESCE(last_seen_at,NOW()), created_at, updated_at FROM devices ORDER BY updated_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.IMEI, &d.Protocol, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) GetDevice(ctx context.Context, imei string) (models.Device, error) {
	var d models.Device
	err := s.Pool.QueryRow(ctx, `SELECT id, imei, protocol, COALESCE(last_seen_at,NOW()), created_at, updated_at FROM devices WHERE imei=$1`, imei).Scan(&d.ID, &d.IMEI, &d.Protocol, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
func (s *Store) GetLatestTelemetry(ctx context.Context, imei string) (models.Telemetry, error) {
	var t models.Telemetry
	var packetType string
	err := s.Pool.QueryRow(ctx, `SELECT id, imei, protocol, packet_type, COALESCE(serial,''), event_io_id, gps_valid, latitude, longitude, speed_kph, course, satellites, mcc, mnc, lac, cell_id, battery_level, signal_level, COALESCE(alarm_code,''), COALESCE(raw_payload,''), device_time, received_at FROM telemetry WHERE imei=$1 ORDER BY received_at DESC LIMIT 1`, imei).Scan(&t.ID, &t.DeviceIMEI, &t.Protocol, &packetType, &t.Serial, &t.EventIOID, &t.GPSValid, &t.Latitude, &t.Longitude, &t.SpeedKPH, &t.Course, &t.Satellites, &t.MCC, &t.MNC, &t.LAC, &t.CellID, &t.BatteryLevel, &t.SignalLevel, &t.AlarmCode, &t.RawPayload, &t.DeviceTime, &t.ReceivedAt)
	if err != nil {
		return t, err
	}
	t.PacketType = models.PacketType(packetType)
	return t, nil
}
func (s *Store) QueueCommand(ctx context.Context, imei, protocol, commandText string) (models.CommandOutbox, error) {
	var c models.CommandOutbox
	var status string
	err := s.Pool.QueryRow(ctx, `INSERT INTO command_outbox (imei, protocol, command_text, status) VALUES ($1,$2,$3,'queued') RETURNING id, imei, protocol, command_text, status, COALESCE(error_message,''), created_at, sent_at, acked_at`, imei, protocol, commandText).Scan(&c.ID, &c.IMEI, &c.Protocol, &c.CommandText, &status, &c.ErrorMessage, &c.CreatedAt, &c.SentAt, &c.AckedAt)
	c.Status = models.CommandStatus(status)
	return c, err
}
func (s *Store) FetchPendingCommands(ctx context.Context, imei, protocol string, limit int) ([]models.CommandOutbox, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, imei, protocol, command_text, status, COALESCE(error_message,''), created_at, sent_at, acked_at FROM command_outbox WHERE imei=$1 AND protocol=$2 AND status='queued' ORDER BY created_at ASC LIMIT $3`, imei, protocol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CommandOutbox
	for rows.Next() {
		var c models.CommandOutbox
		var status string
		if err := rows.Scan(&c.ID, &c.IMEI, &c.Protocol, &c.CommandText, &status, &c.ErrorMessage, &c.CreatedAt, &c.SentAt, &c.AckedAt); err != nil {
			return nil, err
		}
		c.Status = models.CommandStatus(status)
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) MarkCommandSent(ctx context.Context, id int64, sentAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE command_outbox SET status='sent', sent_at=$2, updated_at=NOW() WHERE id=$1`, id, sentAt)
	return err
}
func (s *Store) MarkCommandAckedLatest(ctx context.Context, imei, protocol string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE command_outbox SET status='acked', acked_at=NOW(), updated_at=NOW() WHERE id=(SELECT id FROM command_outbox WHERE imei=$1 AND protocol=$2 AND status='sent' ORDER BY sent_at DESC NULLS LAST, id DESC LIMIT 1)`, imei, protocol)
	return err
}
func (s *Store) MarkCommandFailed(ctx context.Context, id int64, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE command_outbox SET status='failed', error_message=$2, updated_at=NOW() WHERE id=$1`, id, msg)
	return err
}
func (s *Store) ListCommands(ctx context.Context, imei, status string, limit int) ([]models.CommandOutbox, error) {
	q := `SELECT id, imei, protocol, command_text, status, COALESCE(error_message,''), created_at, sent_at, acked_at FROM command_outbox WHERE ($1='' OR imei=$1) AND ($2='' OR status=$2) ORDER BY id DESC LIMIT $3`
	rows, err := s.Pool.Query(ctx, q, imei, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CommandOutbox
	for rows.Next() {
		var c models.CommandOutbox
		var st string
		if err := rows.Scan(&c.ID, &c.IMEI, &c.Protocol, &c.CommandText, &st, &c.ErrorMessage, &c.CreatedAt, &c.SentAt, &c.AckedAt); err != nil {
			return nil, err
		}
		c.Status = models.CommandStatus(st)
		out = append(out, c)
	}
	return out, rows.Err()
}
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nonZeroTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
