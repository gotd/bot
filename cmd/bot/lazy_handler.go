package main

import (
	"context"
	"sync"

	"go.uber.org/atomic"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type LazyHandler struct {
	next telegram.UpdateHandler
	init atomic.Bool
	once sync.Once
}

func (lz *LazyHandler) Handle(ctx context.Context, u tg.UpdatesClass) error {
	if !lz.init.Load() {
		return nil
	}

	return lz.next.Handle(ctx, u)
}

func (lz *LazyHandler) Init(next telegram.UpdateHandler) {
	lz.once.Do(func() {
		lz.next = next
		lz.init.Store(true)
	})
}
