package main

import (
	"sync"

	"github.com/cockroachdb/pebble"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/bin"
)

// State represents current state.
type State struct {
	mux sync.Mutex
	pts int

	db  *pebble.DB
	log *zap.Logger
}

type StateUpdate struct {
	Remote int
	Local  int
}

// Commit offset, like in Kafka.
func (s *State) Commit(pts int) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.pts >= pts {
		return nil
	}

	if err := s.set(pts); err != nil {
		return xerrors.Errorf("set: %w", err)
	}
	s.pts = pts

	s.log.Debug("Commit", zap.Int("pts", pts))

	return nil
}

// Sync to remote pts. If not observed, applyUpdate is called.
func (s *State) Sync(remoteTimeStamp int, applyUpdate func(upd StateUpdate) error) error {
	s.mux.Lock()
	syncNeeded := s.pts < remoteTimeStamp
	s.mux.Unlock()

	if s.pts == 0 {
		// Got initial state.
		if err := s.Commit(remoteTimeStamp); err != nil {
			return xerrors.Errorf("commit init state: %w", err)
		}
		return nil
	}

	if !syncNeeded {
		return nil
	}

	if err := applyUpdate(StateUpdate{Remote: remoteTimeStamp, Local: s.pts}); err != nil {
		return xerrors.Errorf("apply: %w", err)
	}
	if err := s.Commit(remoteTimeStamp); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}

	return nil
}

func (s *State) set(pts int) error {
	var b bin.Buffer
	b.PutInt(pts)
	if err := s.db.Set([]byte("pts"), b.Buf, nil); err != nil {
		return xerrors.Errorf("put: %w", err)
	}

	s.pts = pts
	s.log.Info("Updated local state", zap.Int("pts", pts))

	return nil
}

func (s *State) Load() error {
	v, closer, err := s.db.Get([]byte("pts"))
	if xerrors.Is(err, pebble.ErrNotFound) {
		// No state.
		s.pts = 0
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get: %w", err)
	}
	defer func() { _ = closer.Close() }()

	b := bin.Buffer{Buf: v}
	n, err := b.Int()
	if err != nil {
		return xerrors.Errorf("failed to get long")
	}
	s.pts = n

	return nil
}
