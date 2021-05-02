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

func (d loggedDispatcher) Handle(ctx context.Context, u tg.UpdatesClass) error {
	d.log.Debug("Update",
		zap.String("t", fmt.Sprintf("%T", u)),
	)
	return d.UpdateHandler.Handle(ctx, u)
}
