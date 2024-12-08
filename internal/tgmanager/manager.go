package tgmanager

import (
	"context"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/gotd/bot/internal/ent"
)

// Manager manages telegram test accounts.
type Manager struct {
	log    *zap.Logger
	db     *ent.Client
	meter  metric.Meter
	tracer trace.Tracer

	accounts map[string]*Account
	mux      sync.Mutex
}

func NewManager(log *zap.Logger, db *ent.Client, meterProvider metric.MeterProvider, tracerProvider trace.TracerProvider) (*Manager, error) {
	meter := meterProvider.Meter("bot.gotd.dev/tgmanager")
	tracer := tracerProvider.Tracer("bot.gotd.dev/tgmanager")
	return &Manager{
		log:      log,
		db:       db,
		meter:    meter,
		tracer:   tracer,
		accounts: make(map[string]*Account),
	}, nil
}

func (m *Manager) tick(baseCtx context.Context) (rerr error) {
	ctx, cancel := context.WithTimeout(baseCtx, time.Minute)
	defer cancel()

	ctx, span := m.tracer.Start(ctx, "tick")
	defer func() {
		if rerr != nil {
			span.RecordError(rerr)
		}
		span.End()
	}()

	accounts, err := m.db.TelegramAccount.Query().All(ctx)
	if err != nil {
		return errors.Wrap(err, "query accounts")
	}
	for _, account := range accounts {
		if _, ok := m.accounts[account.ID]; ok {
			continue
		}
		lg := m.log.With(zap.String("phone", account.ID))
		a := NewAccount(lg, m.db, m.tracer, account.ID)

		m.mux.Lock()
		m.accounts[account.ID] = a
		m.mux.Unlock()

		go func() {
			lg.Info("Starting account runner")
			defer func() {
				m.mux.Lock()
				delete(m.accounts, account.ID)
				m.mux.Unlock()
			}()
			if err := a.Run(baseCtx); err != nil {
				lg.Error("Account run failed", zap.Error(err))
			}
		}()
	}
	return nil
}

func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	m.log.Info("Starting manager")
	defer func() {
		m.log.Info("Stopping manager")
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := m.tick(ctx); err != nil {
				m.log.Error("Tick failed", zap.Error(err))
			}
		}
	}
}
