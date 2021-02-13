package main

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type loggedDispatcher struct {
	telegram.UpdateHandler
	log *zap.Logger
}

func (d loggedDispatcher) Handle(ctx context.Context, updates *tg.Updates) error {
	for _, u := range updates.Updates {
		d.log.Debug("Update",
			zap.String("t", fmt.Sprintf("%T", u)),
		)
	}
	return d.UpdateHandler.Handle(ctx, updates)
}

func (d loggedDispatcher) HandleShort(ctx context.Context, u *tg.UpdateShort) error {
	d.log.Debug("UpdateShort",
		zap.String("t", fmt.Sprintf("%T", u.Update)),
	)
	return d.UpdateHandler.HandleShort(ctx, u)
}
