package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"gps-listener-backend/internal/app"
	"gps-listener-backend/internal/config"
	"gps-listener-backend/internal/protocol/gt06"
	"gps-listener-backend/internal/runtime"
)

type service struct{}

func main() { app.Run("gt06-listener", &service{}) }
func (s *service) HandleConnection(ctx context.Context, conn net.Conn) {
	remote := conn.RemoteAddr().String()
	log.Printf("[gt06][%s] connected", remote)
	defer log.Printf("[gt06][%s] disconnected", remote)
	store := config.StoreFromContext(ctx)
	reader := bufio.NewReader(conn)
	imei := ""
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		head, err := reader.Peek(3)
		if err != nil {
			if isClosedErr(err) {
				return
			}
			log.Printf("[gt06][%s] peek error: %v", remote, err)
			return
		}
		if len(head) < 3 || head[0] != 0x78 || head[1] != 0x78 {
			log.Printf("[gt06][%s] invalid frame start", remote)
			return
		}
		frameLen := int(head[2]) + 5
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(reader, frame); err != nil {
			log.Printf("[gt06][%s] read error: %v", remote, err)
			return
		}
		_ = store.InsertRawPacket(ctx, imei, "gt06", "uplink", frame)
		pkt, err := gt06.Parse(frame)
		if err != nil {
			log.Printf("[gt06][%s] parse error: %v", remote, err)
			continue
		}
		if pkt.IMEI != "" {
			imei = pkt.IMEI
			_ = store.UpsertDevice(ctx, imei, "gt06")
		}
		if pkt.Telemetry != nil {
			if pkt.Telemetry.DeviceIMEI == "" {
				pkt.Telemetry.DeviceIMEI = imei
			}
			_ = store.InsertTelemetry(ctx, *pkt.Telemetry)
		}
		if pkt.ProtocolNumber == gt06.ProtoLogin || pkt.ProtocolNumber == gt06.ProtoStatus {
			ack := gt06.BuildAck(pkt.ProtocolNumber, pkt.Serial)
			if _, err := conn.Write(ack); err == nil {
				_ = store.InsertRawPacket(ctx, imei, "gt06", "downlink", ack)
			}
		}
		if pkt.ProtocolNumber == gt06.ProtoString {
			_ = store.MarkCommandAckedLatest(ctx, imei, "gt06")
		}
		runtime.DrainPendingCommands(ctx, store, imei, "gt06", "", conn)
		log.Printf("[gt06][%s] proto=0x%02X serial=%04X imei=%s raw=%s", remote, pkt.ProtocolNumber, pkt.Serial, imei, strings.ToUpper(hex.EncodeToString(frame)))
	}
}
func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "closed") || strings.Contains(s, "reset by peer") || strings.Contains(s, "timeout")
}
