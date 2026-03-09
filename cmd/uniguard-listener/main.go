package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"gps-listener-backend/internal/app"
	"gps-listener-backend/internal/config"
	"gps-listener-backend/internal/protocol/uniguard"
	"gps-listener-backend/internal/runtime"
)

type service struct{}

func main() { app.Run("uniguard-listener", &service{}) }
func (s *service) HandleConnection(ctx context.Context, conn net.Conn) {
	remote := conn.RemoteAddr().String()
	log.Printf("[uniguard][%s] connected", remote)
	defer log.Printf("[uniguard][%s] disconnected", remote)
	store := config.StoreFromContext(ctx)
	reader := bufio.NewReader(conn)
	imei := ""
	serialHint := "0001"
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		data, err := reader.ReadBytes('$')
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Printf("[uniguard][%s] read error: %v", remote, err)
			return
		}
		data = bytes.TrimSpace(data)
		_ = store.InsertRawPacket(ctx, imei, "uniguard", "uplink", data)
		frame, err := uniguard.Parse(string(data))
		if err != nil {
			log.Printf("[uniguard][%s] parse error: %v", remote, err)
			continue
		}
		imei = frame.IMEI
		serialHint = frame.Serial
		_ = store.UpsertDevice(ctx, imei, "uniguard")
		if frame.Telemetry != nil {
			_ = store.InsertTelemetry(ctx, *frame.Telemetry)
		}
		upper := strings.ToUpper(frame.Content)
		var ack string
		if strings.Contains(upper, "LOCA:") {
			ack = uniguard.BuildAckLOCA(frame.ID, frame.IMEI, frame.Serial)
		}
		if strings.Contains(upper, "SYNC") {
			ack = uniguard.BuildAckSYNC(frame.ID, frame.IMEI, frame.Serial, time.Now())
		}
		if ack != "" {
			if _, err := conn.Write([]byte(ack)); err == nil {
				_ = store.InsertRawPacket(ctx, frame.IMEI, "uniguard", "downlink", []byte(ack))
			}
		}
		if strings.HasPrefix(strings.ToUpper(frame.Content), "RET,") {
			_ = store.MarkCommandAckedLatest(ctx, imei, "uniguard")
		}
		runtime.DrainPendingCommands(ctx, store, imei, "uniguard", serialHint, conn)
		log.Printf("[uniguard][%s] imei=%s serial=%s content=%s", remote, frame.IMEI, frame.Serial, frame.Content)
	}
}
