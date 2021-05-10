package main

import (
	"encoding/binary"

	"github.com/gotd/contrib/updates"
	bolt "go.etcd.io/bbolt"
)

func i2b(v int) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, uint64(v)); return b }

func b2i(b []byte) int { return int(binary.LittleEndian.Uint64(b)) }

var _ updates.Storage = (*BoltState)(nil)

type BoltState struct {
	db *bolt.DB
}

func NewBoltState(db *bolt.DB) *BoltState { return &BoltState{db} }

func (s *BoltState) GetState() (updates.State, error) {
	tx, err := s.db.Begin(false)
	if err != nil {
		return updates.State{}, err
	}
	defer func() { _ = tx.Rollback() }()

	state := tx.Bucket([]byte("state"))
	if state == nil {
		return updates.State{}, updates.ErrStateNotFound
	}

	var (
		pts  = state.Get([]byte("pts"))
		qts  = state.Get([]byte("qts"))
		date = state.Get([]byte("date"))
		seq  = state.Get([]byte("seq"))
	)

	if pts == nil || qts == nil || date == nil || seq == nil {
		return updates.State{}, updates.ErrStateNotFound
	}

	return updates.State{
		Pts:  b2i(pts),
		Qts:  b2i(qts),
		Date: b2i(date),
		Seq:  b2i(seq),
	}, nil
}

func (s *BoltState) SetState(state updates.State) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}

		check := func(e error) {
			if err != nil {
				return
			}
			err = e
		}

		check(b.Put([]byte("pts"), i2b(state.Pts)))
		check(b.Put([]byte("qts"), i2b(state.Qts)))
		check(b.Put([]byte("date"), i2b(state.Date)))
		check(b.Put([]byte("seq"), i2b(state.Seq)))
		return err
	})
}

func (s *BoltState) SetPts(pts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}
		return b.Put([]byte("pts"), i2b(pts))
	})
}

func (s *BoltState) SetQts(qts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}
		return b.Put([]byte("qts"), i2b(qts))
	})
}

func (s *BoltState) SetDateSeq(date, seq int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}

		if err := b.Put([]byte("date"), i2b(date)); err != nil {
			return err
		}
		return b.Put([]byte("seq"), i2b(seq))
	})
}

func (s *BoltState) SetChannelPts(channelID, pts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("channels"))
		if err != nil {
			return err
		}
		return b.Put(i2b(channelID), i2b(pts))
	})
}

func (s *BoltState) Channels(iter func(channelID, pts int)) error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("channels"))
		if b == nil {
			return nil
		}

		return b.ForEach(func(k, v []byte) error {
			iter(b2i(k), b2i(v))
			return nil
		})
	})
}

func (s *BoltState) ForgetAll() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("state")) != nil {
			if err := tx.DeleteBucket([]byte("state")); err != nil {
				return err
			}
		}

		if tx.Bucket([]byte("channels")) != nil {
			if err := tx.DeleteBucket([]byte("channels")); err != nil {
				return err
			}
		}
		return nil
	})
}
