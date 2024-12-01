package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/app"
	"go.uber.org/zap"

	iapp "github.com/gotd/bot/internal/app"
)

func main() {
	app.Run(func(ctx context.Context, lg *zap.Logger, m *app.Metrics) error {
		mx := &iapp.Metrics{}
		{
			var err error
			meter := m.MeterProvider().Meter("gotd.bot")
			if mx.Bytes, err = meter.Int64Counter("gotd.bot.bytes"); err != nil {
				return errors.Wrap(err, "bytes")
			}
			if mx.Messages, err = meter.Int64Counter("gotd.bot.messages"); err != nil {
				return errors.Wrap(err, "messages")
			}
			if mx.Responses, err = meter.Int64Counter("gotd.bot.responses"); err != nil {
				return errors.Wrap(err, "responses")
			}
		}
		return runBot(ctx, m, mx, lg.Named("bot"))
	})
}
