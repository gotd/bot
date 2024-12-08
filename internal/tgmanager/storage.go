package tgmanager

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/gotd/td/session"

	"github.com/gotd/bot/internal/ent"
)

type SessionStorage struct {
	db *ent.Client
	id string
}

func (s SessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	acc, err := s.db.TelegramAccount.Get(ctx, s.id)
	if err != nil {
		return nil, errors.Wrap(err, "get account")
	}
	if acc.SessionData == nil || len(*acc.SessionData) == 0 {
		return nil, session.ErrNotFound
	}
	return *acc.SessionData, nil
}

func (s SessionStorage) StoreSession(ctx context.Context, data []byte) error {
	_, err := s.db.TelegramAccount.UpdateOneID(s.id).
		SetSessionData(data).
		Save(ctx)
	if err != nil {
		return errors.Wrap(err, "update session")
	}
	return err
}
