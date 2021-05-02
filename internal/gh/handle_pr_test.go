package gh

import (
	"context"
	"strconv"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/google/go-github/v33/github"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/xerrors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/bot/internal/storage"
)

type mockResolver map[string]tg.InputPeerClass

func (m mockResolver) ResolveDomain(ctx context.Context, domain string) (tg.InputPeerClass, error) {
	f, ok := m[domain]
	if !ok {
		return nil, tgerr.New(400, tg.ErrUsernameInvalid)
	}
	return f, nil
}

func (m mockResolver) ResolvePhone(ctx context.Context, phone string) (tg.InputPeerClass, error) {
	f, ok := m[phone]
	if !ok {
		return nil, tgerr.New(400, tg.ErrUsernameInvalid)
	}
	return f, nil
}

func prEvent(prID int, orgID int64) *github.PullRequestEvent {
	return &github.PullRequestEvent{
		PullRequest: &github.PullRequest{
			Merged: github.Bool(true),
			Number: &prID,
		},
		Repo: &github.Repository{
			ID:   &orgID,
			Name: github.String("test"),
		},
	}
}

type mockInvoker struct {
	lastReq *tg.MessagesEditMessageRequest
}

func (m *mockInvoker) InvokeRaw(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesEditMessageRequest)
	if !ok {
		return xerrors.Errorf("unexpected type %T", input)
	}
	m.lastReq = req
	return nil
}

func TestWebhook(t *testing.T) {
	ctx := context.Background()
	a := require.New(t)

	msgID, lastMsgID := 10, 11
	prID, orgID := 13, int64(37)
	channel := &tg.InputPeerChannel{
		ChannelID:  69,
		AccessHash: 42,
	}

	log := zaptest.NewLogger(t)
	db, err := pebble.Open("golovach_lena.db", &pebble.Options{FS: vfs.NewMem()})
	a.NoError(err)

	err = db.Set(
		storage.LastMsgIDKey(channel.ChannelID),
		strconv.AppendInt(nil, int64(lastMsgID), 10),
		pebble.Sync,
	)
	a.NoError(err)

	err = db.Set(
		PRMsgIDKey(prEvent(prID, orgID)),
		strconv.AppendInt(nil, int64(msgID), 10),
		pebble.Sync,
	)
	a.NoError(err)

	invoker := &mockInvoker{}
	raw := tg.NewClient(invoker)
	sender := message.NewSender(raw).WithResolver(mockResolver{
		"gotd_ru": channel,
	})
	hook := NewWebhook(db, sender, "secret").WithLogger(log)

	event := prEvent(prID, orgID)
	err = hook.handlePRClosed(ctx, event)
	a.NoError(err)

	a.NotNil(invoker.lastReq)
	a.Contains(invoker.lastReq.Message, "opened")
}
