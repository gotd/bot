package storage

import (
	"context"
	"strconv"

	"github.com/cockroachdb/pebble"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/bot/internal/dispatch"
)

// Hook is event handler which saves last message ID of dialog to the Pebble storage.
type Hook struct {
	next dispatch.MessageHandler
	db   *pebble.DB
}

// LastMsgIDKey generates last message ID key for given channel.
func LastMsgIDKey(channelID int) []byte {
	return strconv.AppendInt([]byte("last_msg_"), int64(channelID), 10)
}

// NewHook creates new hook.
func NewHook(next dispatch.MessageHandler, db *pebble.DB) Hook {
	return Hook{next: next, db: db}
}

func (h Hook) update(peerID, msgID int) (rerr error) {
	key := LastMsgIDKey(peerID)

	b := h.db.NewIndexedBatch()
	data, closer, err := b.Get(key)
	switch {
	case xerrors.Is(err, pebble.ErrNotFound):
	case err != nil:
		return err
	default:
		defer func() {
			multierr.AppendInto(&rerr, closer.Close())
		}()
		s := string(data)
		id, err := strconv.Atoi(s)
		if err != nil {
			return xerrors.Errorf("parse msg id %q: %w", s, err)
		}

		if id > msgID {
			return nil
		}
	}

	if err := b.Set(key, strconv.AppendInt(nil, int64(msgID), 10), pebble.Sync); err != nil {
		return xerrors.Errorf("set msg_id %d: %w", peerID, err)
	}

	if err := b.Commit(nil); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}

	return nil
}

// OnMessage implements dispatch.MessageHandler.
func (h Hook) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	ch, ok := e.Channel()
	if !ok {
		return h.next.OnMessage(ctx, e)
	}

	return multierr.Append(
		h.update(ch.ID, e.Message.ID),
		h.next.OnMessage(ctx, e),
	)
}
