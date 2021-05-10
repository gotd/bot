package main

import (
	"context"

	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/gentext"
	"github.com/gotd/bot/internal/gh"
	"github.com/gotd/bot/internal/gpt"
	"github.com/gotd/bot/internal/inspect"
	"github.com/gotd/bot/internal/metrics"
	"github.com/gotd/bot/internal/tts"
)

func setupBot(app *App) error {
	app.mux.HandleFunc("/bot", "Ping bot", func(ctx context.Context, e dispatch.MessageEvent) error {
		_, err := e.Reply().Text(ctx, "What?")
		return err
	})
	app.mux.HandleFunc("/dice", "Send dice",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Dice(ctx)
			return err
		})
	app.mux.HandleFunc("/darts", "Send darts",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Darts(ctx)
			return err
		})
	app.mux.HandleFunc("/basketball", "Send basketball",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Basketball(ctx)
			return err
		})
	app.mux.HandleFunc("/football", "Send football",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Football(ctx)
			return err
		})
	app.mux.HandleFunc("/casino", "Send casino",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Casino(ctx)
			return err
		})
	app.mux.HandleFunc("/bowling", "Send bowling",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Bowling(ctx)
			return err
		})

	app.mux.Handle("/pp", "Pretty print replied message", inspect.Pretty())
	app.mux.Handle("/json", "Print JSON of replied message", inspect.JSON())
	app.mux.Handle("/stat", "Metrics and version", metrics.NewHandler(app.mts))
	app.mux.Handle("/tts", "Text to speech", tts.New(app.http))
	app.mux.Handle("/gpt2", "Complete text with GPT2",
		gpt.New(gentext.NewGPT2().WithClient(app.http)))
	app.mux.Handle("/gpt3", "Complete text with GPT3",
		gpt.New(gentext.NewGPT3().WithClient(app.http)))

	if app.github != nil {
		app.mux.Handle("github", "", gh.New(app.github))
	}
	return nil
}
