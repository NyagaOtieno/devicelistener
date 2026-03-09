# GPS listener microservices

Production-style Go backend for:
- GT06
- UniGuard
- Teltonika FMB920

Services:
- `gt06-listener` on `9001`
- `uniguard-listener` on `9002`
- `teltonika-listener` on `9003`
- `api` on `8080`

## What is included
- TCP listeners as separate microservices
- PostgreSQL schema
- raw packet storage
- normalized telemetry storage
- HTTP API for devices, latest telemetry, and command queueing
- queued command dispatch to connected devices
- Docker Compose for local startup

## API
- `GET /health`
- `GET /devices`
- `GET /devices/{imei}` -> latest telemetry
- `POST /devices/{imei}/commands`
- `GET /commands?imei=&status=`

Example:
```bash
curl -X POST http://localhost:8080/devices/123456789012345/commands \
  -H "Content-Type: application/json" \
  -d '{"protocol":"teltonika-fmb920","command":"getinfo"}'
```

## Teltonika notes
The listener accepts the standard TCP handshake where the device first sends `2-byte IMEI length + ASCII IMEI`, then the server replies with binary `0x01`. AVL packets use a `00000000` preamble, `data field length`, codec id, record counters, and CRC, and the server acknowledges accepted records with a 4-byte integer. Codec 8 (`0x08`) and Codec 8 Extended (`0x8E`) are the main FMB920 data paths; Teltonika documents also describe Codec 12 for GPRS commands. citeturn0search0turn0search1turn1search1turn1search13

## GT06 notes
GT06 frames start with `0x78 0x78`, include a packet length, protocol number, serial number, CRC-ITU, and stop bits `0x0D 0x0A`. Login is `0x01`, location is `0x12`, status/heartbeat is `0x13`, alarm is `0x16`, server commands use `0x80`, and terminal string replies use `0x15`. The server is expected to answer login and heartbeat packets, and GT06 command contents are ASCII-compatible with text-message commands. fileciteturn2file2L17-L30 fileciteturn3file4L17-L30 fileciteturn2file6L38-L56 fileciteturn3file3L21-L32

## UniGuard notes
UniGuard uses text frames like `ID#IMEI#serial#length#content$`. Position packages use `LOCA`, heartbeat/sync packages use `SYNC`, downstream acknowledgements are `ACK^LOCA` and `ACK^SYNC,<utc time>`, and command replies use `RET,...`. fileciteturn2file9L45-L55 fileciteturn2file0L39-L41 fileciteturn3file7L14-L18

## Run
```bash
cp .env.example .env
docker compose up --build
```

## Direct build
```bash
go mod tidy
go build -o bin/gt06 ./cmd/gt06-listener
go build -o bin/uniguard ./cmd/uniguard-listener
go build -o bin/teltonika ./cmd/teltonika-listener
go build -o bin/api ./cmd/api
```
