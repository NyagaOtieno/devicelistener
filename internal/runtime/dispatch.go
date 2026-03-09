package runtime

import (
	"context"
	"io"
	"log"
	"time"

	"gps-listener-backend/internal/command"
	"gps-listener-backend/internal/storage"
)

func DrainPendingCommands(ctx context.Context, store *storage.Store, imei, protocol string, serialHint string, w io.Writer) {
	if store == nil || imei == "" || w == nil {
		return
	}
	cmds, err := store.FetchPendingCommands(ctx, imei, protocol, 10)
	if err != nil || len(cmds) == 0 {
		return
	}
	for _, cmd := range cmds {
		payload, err := command.Build(protocol, imei, serialHint, cmd.CommandText)
		if err != nil {
			_ = store.MarkCommandFailed(ctx, cmd.ID, err.Error())
			continue
		}
		if _, err := w.Write(payload); err != nil {
			_ = store.MarkCommandFailed(ctx, cmd.ID, err.Error())
			log.Printf("dispatch %d failed: %v", cmd.ID, err)
			return
		}
		_ = store.InsertRawPacket(ctx, imei, protocol, "downlink", payload)
		_ = store.MarkCommandSent(ctx, cmd.ID, time.Now().UTC())
	}
}
