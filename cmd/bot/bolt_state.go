package main

import (
	"encoding/binary"
	"fmt"

	bolt "go.etcd.io/bbolt"

	"github.com/gotd/td/telegram/updates"
)

func i2b(v int) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, uint64(v)); return b }

func b2i(b []byte) int { return int(binary.LittleEndian.Uint64(b)) }

var _ updates.StateStorage = (*BoltState)(nil)

type BoltState struct {
	db *bolt.DB
}

func NewBoltState(db *bolt.DB) *BoltState { return &BoltState{db} }

func (s *BoltState) GetState(userID int) (state updates.State, found bool, err error) {
	tx, err := s.db.Begin(false)
	if err != nil {
		return updates.State{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	user := tx.Bucket(i2b(userID))
	if user == nil {
		return updates.State{}, false, nil
	}

	stateBucket := user.Bucket([]byte("state"))
	if stateBucket == nil {
		return updates.State{}, false, nil
	}

	var (
		pts  = stateBucket.Get([]byte("pts"))
		qts  = stateBucket.Get([]byte("qts"))
		date = stateBucket.Get([]byte("date"))
		seq  = stateBucket.Get([]byte("seq"))
	)

	if pts == nil || qts == nil || date == nil || seq == nil {
		return updates.State{}, false, nil
	}

	return updates.State{
		Pts:  b2i(pts),
		Qts:  b2i(qts),
		Date: b2i(date),
		Seq:  b2i(seq),
	}, true, nil
}

func (s *BoltState) SetState(userID int, state updates.State) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		b, err := user.CreateBucketIfNotExists([]byte("state"))
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

func (s *BoltState) SetPts(userID, pts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		state := user.Bucket([]byte("state"))
		if state == nil {
			return fmt.Errorf("state not found")
		}
		return state.Put([]byte("pts"), i2b(pts))
	})
}

func (s *BoltState) SetQts(userID, qts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		state := user.Bucket([]byte("state"))
		if state == nil {
			return fmt.Errorf("state not found")
		}
		return state.Put([]byte("qts"), i2b(qts))
	})
}

func (s *BoltState) SetDate(userID, date int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		state := user.Bucket([]byte("state"))
		if state == nil {
			return fmt.Errorf("state not found")
		}
		return state.Put([]byte("date"), i2b(date))
	})
}

func (s *BoltState) SetSeq(userID, seq int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		state := user.Bucket([]byte("state"))
		if state == nil {
			return fmt.Errorf("state not found")
		}
		return state.Put([]byte("seq"), i2b(seq))
	})
}

func (s *BoltState) SetDateSeq(userID, date, seq int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		state := user.Bucket([]byte("state"))
		if state == nil {
			return fmt.Errorf("state not found")
		}
		if err := state.Put([]byte("date"), i2b(date)); err != nil {
			return err
		}
		return state.Put([]byte("seq"), i2b(seq))
	})
}

func (s *BoltState) GetChannelPts(userID, channelID int) (int, bool, error) {
	tx, err := s.db.Begin(false)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()

	user := tx.Bucket(i2b(userID))
	if user == nil {
		return 0, false, nil
	}

	channels := user.Bucket([]byte("channels"))
	if channels == nil {
		return 0, false, nil
	}

	pts := channels.Get(i2b(channelID))
	if pts == nil {
		return 0, false, nil
	}

	return b2i(pts), true, nil
}

func (s *BoltState) SetChannelPts(userID, channelID, pts int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		channels, err := user.CreateBucketIfNotExists([]byte("channels"))
		if err != nil {
			return err
		}
		return channels.Put(i2b(channelID), i2b(pts))
	})
}

func (s *BoltState) ForEachChannels(userID int, f func(channelID, pts int) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		user, err := tx.CreateBucketIfNotExists(i2b(userID))
		if err != nil {
			return err
		}

		channels, err := user.CreateBucketIfNotExists([]byte("channels"))
		if err != nil {
			return err
		}

		return channels.ForEach(func(k, v []byte) error {
			return f(b2i(k), b2i(v))
		})
	})
}
