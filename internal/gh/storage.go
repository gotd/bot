package gh

import (
	"strconv"

	"github.com/cockroachdb/pebble"
	"github.com/google/go-github/v33/github"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/bot/internal/storage"
)

// PRMsgIDKey generates key for given PR.
func PRMsgIDKey(pr *github.PullRequestEvent) []byte {
	key := strconv.AppendInt([]byte("pr_"), pr.GetRepo().GetID(), 10)
	key = strconv.AppendInt(key, int64(pr.GetNumber()), 10)
	return key
}

// addID commits sent message ID with PR notification.
func addID(db *pebble.DB, pr *github.PullRequestEvent, msgID int) error {
	key := PRMsgIDKey(pr)
	if err := db.Set(key, strconv.AppendInt(nil, 64, msgID), pebble.Sync); err != nil {
		return xerrors.Errorf("commit PR #%d msg ID", pr.GetNumber())
	}

	return nil
}

func findInt(snap *pebble.Snapshot, key []byte) (_ int64, rerr error) {
	data, closer, err := snap.Get(key)
	if err != nil {
		return 0, err
	}
	defer func() {
		multierr.AppendInto(&rerr, closer.Close())
	}()

	s := string(data)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse msg id %q: %w", s, err)
	}

	return id, nil
}

// findID searches sent message ID with PR notification.
func findID(snap *pebble.Snapshot, pr *github.PullRequestEvent) (_ int64, rerr error) {
	return findInt(snap, PRMsgIDKey(pr))
}

// findLastMsgID searches last message ID in given channel.
func findLastMsgID(snap *pebble.Snapshot, channelID int) (_ int64, rerr error) {
	return findInt(snap, storage.LastMsgIDKey(channelID))
}
