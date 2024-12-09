package tgmanager

import (
	"context"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/google/uuid"
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
	leases   map[string]*Lease
	mux      sync.Mutex
}

var ErrNoLease = errors.New("no accounts available")

// LeaseCode returns account code for lease.
// If account is not leased, returns ErrNoLease.
// If code is not received yet, returns empty string.
func (m *Manager) LeaseCode(ctx context.Context, token uuid.UUID) (string, error) {
	ctx, span := m.tracer.Start(ctx, "LeaseCode")
	defer func() {
		span.End()
	}()

	m.mux.Lock()
	var lease *Lease
	for _, l := range m.leases {
		if l.Token != token {
			continue
		}
		lease = l
	}
	m.mux.Unlock()

	if lease == nil {
		return "", errors.Wrap(ErrNoLease, "no account with token")
	}

	acc, err := m.db.TelegramAccount.Get(ctx, lease.Account)
	if err != nil {
		return "", errors.Wrap(err, "get account")
	}

	if acc.CodeAt == nil || acc.Code == nil {
		return "", nil
	}
	if acc.CodeAt.Before(lease.Start) {
		return "", nil
	}

	return *acc.Code, nil
}

// Heartbeat updates lease expiration time.
func (m *Manager) Heartbeat(token uuid.UUID) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	for _, lease := range m.leases {
		if lease.Token != token {
			continue
		}
		lease.Until = time.Now().Add(time.Second * 15)
		return nil
	}
	return errors.Wrap(ErrNoLease, "no account with token")
}

// Forget lease.
func (m *Manager) Forget(token uuid.UUID) {
	m.mux.Lock()
	defer m.mux.Unlock()

	var toForget string
	for phone, lease := range m.leases {
		if lease.Token == token {
			toForget = phone
			break
		}
	}
	if toForget == "" {
		return
	}
	delete(m.leases, toForget)
}

// Acquire new lease.
func (m *Manager) Acquire() (*Lease, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if len(m.accounts) == 0 {
		return nil, errors.Wrap(ErrNoLease, "no accounts")
	}

	for phone := range m.accounts {
		if _, ok := m.leases[phone]; ok {
			// Already leased.
			continue
		}
		lease := &Lease{
			Account: phone,
			Token:   uuid.New(),
			Start:   time.Now(),
			Until:   time.Now().Add(time.Second * 15),
		}
		m.leases[phone] = lease
		return lease, nil
	}

	return nil, errors.Wrap(ErrNoLease, "all accounts leased")
}

func (m *Manager) tickLease(now time.Time) {
	m.mux.Lock()
	defer m.mux.Unlock()

	var toDelete []string
	for phone, lease := range m.leases {
		if lease.Until.After(now) {
			continue
		}
		m.log.Info("Lease expired",
			zap.String("phone", phone),
			zap.Stringer("token", lease.Token),
		)
		toDelete = append(toDelete, phone)
	}

	for _, phone := range toDelete {
		delete(m.leases, phone)
	}
	m.log.Info("Lease cleanup done",
		zap.Int("deleted", len(toDelete)),
		zap.Int("total", len(m.leases)),
		zap.Int("accounts", len(m.accounts)),
	)
}

// Lease for telegram account.
type Lease struct {
	Account string
	Token   uuid.UUID
	Start   time.Time
	Until   time.Time
}

func NewManager(log *zap.Logger, db *ent.Client, meterProvider metric.MeterProvider, tracerProvider trace.TracerProvider) (*Manager, error) {
	meter := meterProvider.Meter("bot.gotd.dev/tgmanager")
	tracer := tracerProvider.Tracer("bot.gotd.dev/tgmanager")
	mgr := &Manager{
		log:      log,
		db:       db,
		meter:    meter,
		tracer:   tracer,
		accounts: make(map[string]*Account),
		leases:   make(map[string]*Lease),
	}

	accountsTotal, err := meter.Int64ObservableGauge("accounts.total")
	if err != nil {
		return nil, errors.Wrap(err, "create observable gauge")
	}
	accountsLeased, err := meter.Int64ObservableGauge("accounts.leased")
	if err != nil {
		return nil, errors.Wrap(err, "create observable gauge")
	}
	accountsFree, err := meter.Int64ObservableGauge("accounts.free")
	if err != nil {
		return nil, errors.Wrap(err, "create observable gauge")
	}

	if _, err := meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		mgr.mux.Lock()
		defer mgr.mux.Unlock()

		total := int64(len(mgr.accounts))
		leased := int64(len(mgr.leases))
		free := total - leased

		observer.ObserveInt64(accountsTotal, total)
		observer.ObserveInt64(accountsLeased, leased)
		observer.ObserveInt64(accountsFree, free)

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "register callback")
	}

	return mgr, nil
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

	now := time.Now()
	m.tickLease(now)

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
