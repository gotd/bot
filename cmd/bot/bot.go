package main

import (
	"context"

	"github.com/gotd/bot/internal/app"
	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/inspect"
)

func setupBot(a *App) error {
	a.mux.HandleFunc("/bot", "Ping bot", func(ctx context.Context, e dispatch.MessageEvent) error {
		_, err := e.Reply().Text(ctx, "What?")
		return err
	})
	a.mux.HandleFunc("/dice", "Send dice",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Dice(ctx)
			return err
		})
	a.mux.HandleFunc("/darts", "Send darts",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Darts(ctx)
			return err
		})
	a.mux.HandleFunc("/basketball", "Send basketball",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Basketball(ctx)
			return err
		})
	a.mux.HandleFunc("/football", "Send football",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Football(ctx)
			return err
		})
	a.mux.HandleFunc("/casino", "Send casino",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Casino(ctx)
			return err
		})
	a.mux.HandleFunc("/bowling", "Send bowling",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Bowling(ctx)
			return err
		})

	a.mux.Handle("/pp", "Pretty print replied message", inspect.Pretty())
	a.mux.Handle("/json", "Print JSON of replied message", inspect.JSON())
	a.mux.Handle("/stat", "Version", app.NewHandler())
	return nil
}
