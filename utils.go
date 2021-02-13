package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"os/signal"
)

func tokHash(token string) string {
	h := md5.Sum([]byte(token + "gotd-token-salt")) // #nosec
	return fmt.Sprintf("%x", h[:5])
}

func sessionFileName(token string) string {
	return fmt.Sprintf("bot.%s.session.json", tokHash(token))
}

func withSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}
