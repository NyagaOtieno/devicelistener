CREATE TABLE IF NOT EXISTS devices (
    id BIGSERIAL PRIMARY KEY,
    imei VARCHAR(32) NOT NULL UNIQUE,
    protocol VARCHAR(32) NOT NULL,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS raw_packets (
    id BIGSERIAL PRIMARY KEY,
    imei VARCHAR(32),
    protocol VARCHAR(32) NOT NULL,
    direction VARCHAR(16) NOT NULL,
    payload_hex TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telemetry (
    id BIGSERIAL PRIMARY KEY,
    imei VARCHAR(32),
    protocol VARCHAR(32) NOT NULL,
    packet_type VARCHAR(32) NOT NULL,
    serial VARCHAR(32),
    event_io_id INTEGER,
    gps_valid BOOLEAN,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    speed_kph DOUBLE PRECISION,
    course INTEGER,
    satellites INTEGER,
    mcc INTEGER,
    mnc INTEGER,
    lac INTEGER,
    cell_id BIGINT,
    battery_level INTEGER,
    signal_level INTEGER,
    alarm_code VARCHAR(128),
    raw_payload TEXT,
    device_time TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS command_outbox (
    id BIGSERIAL PRIMARY KEY,
    imei VARCHAR(32) NOT NULL,
    protocol VARCHAR(32) NOT NULL,
    command_text TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    acked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telemetry_imei_received_at ON telemetry (imei, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_raw_packets_imei_received_at ON raw_packets (imei, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_telemetry_protocol_received_at ON telemetry (protocol, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_command_outbox_lookup ON command_outbox (imei, protocol, status, created_at ASC);
