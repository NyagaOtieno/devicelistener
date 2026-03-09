package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"gps-listener-backend/internal/app"
	"gps-listener-backend/internal/config"
	"gps-listener-backend/internal/models"
	"gps-listener-backend/internal/protocol/teltonika"
	"gps-listener-backend/internal/runtime"
)

type service struct{}

func main() { app.Run("teltonika-listener", &service{}) }
func (s *service) HandleConnection(ctx context.Context, conn net.Conn) {
	remote := conn.RemoteAddr().String()
	log.Printf("[teltonika][%s] connected", remote)
	defer log.Printf("[teltonika][%s] disconnected", remote)
	store := config.StoreFromContext(ctx)
	reader := bufio.NewReader(conn)
	imei := ""
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		b4, _ := reader.Peek(4)
		if len(b4) < 4 {
			if _, err := reader.Peek(1); err != nil {
				if isClosedErr(err) {
					return
				}
				log.Printf("[teltonika][%s] peek error: %v", remote, err)
				return
			}
		}
		if len(b4) == 4 && binary.BigEndian.Uint32(b4) == 0 {
			header := make([]byte, 8)
			if _, err := io.ReadFull(reader, header); err != nil {
				log.Printf("[teltonika][%s] header read error: %v", remote, err)
				return
			}
			dataLen := int(binary.BigEndian.Uint32(header[4:8]))
			rest := make([]byte, dataLen+4)
			if _, err := io.ReadFull(reader, rest); err != nil {
				log.Printf("[teltonika][%s] body read error: %v", remote, err)
				return
			}
			frame := append(header, rest...)
			_ = store.InsertRawPacket(ctx, imei, "teltonika-fmb920", "uplink", frame)
			if len(frame) > 8 && frame[8] == 0x0C {
				resp, err := teltonika.ParseCodec12(frame)
				if err == nil {
					_ = store.InsertTelemetry(ctx, models.Telemetry{DeviceIMEI: imei, Protocol: "teltonika-fmb920", PacketType: models.PacketTypeCommand, AlarmCode: resp.Text, RawPayload: string(frame), ReceivedAt: time.Now().UTC()})
					_ = store.MarkCommandAckedLatest(ctx, imei, "teltonika-fmb920")
				}
				runtime.DrainPendingCommands(ctx, store, imei, "teltonika-fmb920", "", conn)
				continue
			}
			pkt, err := teltonika.ParseAVL(frame, imei)
			if err != nil {
				log.Printf("[teltonika][%s] parse error: %v", remote, err)
				continue
			}
			for _, rec := range pkt.Records {
				rec.RawPayload = pkt.RawHex
				if rec.DeviceIMEI == "" {
					rec.DeviceIMEI = imei
				}
				_ = store.InsertTelemetry(ctx, rec)
			}
			ack := teltonika.BuildAVLAck(pkt.AcceptedCount)
			if _, err := conn.Write(ack); err == nil {
				_ = store.InsertRawPacket(ctx, imei, "teltonika-fmb920", "downlink", ack)
			}
			runtime.DrainPendingCommands(ctx, store, imei, "teltonika-fmb920", "", conn)
			log.Printf("[teltonika][%s] imei=%s codec=0x%02X records=%d", remote, imei, pkt.CodecID, pkt.RecordCount)
			continue
		}
		hdr, err := reader.Peek(2)
		if err != nil {
			if isClosedErr(err) {
				return
			}
			log.Printf("[teltonika][%s] handshake peek error: %v", remote, err)
			return
		}
		ln := int(binary.BigEndian.Uint16(hdr))
		buf := make([]byte, ln+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			log.Printf("[teltonika][%s] handshake read error: %v", remote, err)
			return
		}
		hs, err := teltonika.ParseHandshake(buf)
		if err != nil {
			log.Printf("[teltonika][%s] handshake parse error: %v", remote, err)
			return
		}
		imei = hs.IMEI
		_ = store.UpsertDevice(ctx, imei, "teltonika-fmb920")
		_ = store.InsertRawPacket(ctx, imei, "teltonika-fmb920", "uplink", buf)
		ack := teltonika.BuildIMEIAck(true)
		if _, err := conn.Write(ack); err == nil {
			_ = store.InsertRawPacket(ctx, imei, "teltonika-fmb920", "downlink", ack)
		}
		_ = store.InsertTelemetry(ctx, models.Telemetry{DeviceIMEI: imei, Protocol: "teltonika-fmb920", PacketType: models.PacketTypeLogin, RawPayload: string(buf), ReceivedAt: time.Now().UTC()})
		runtime.DrainPendingCommands(ctx, store, imei, "teltonika-fmb920", "", conn)
		log.Printf("[teltonika][%s] imei handshake accepted %s", remote, imei)
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
