package gh

import (
	"context"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/go-faster/errors"
	"github.com/google/go-github/v42/github"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

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

func (m *mockInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesEditMessageRequest)
	if !ok {
		return errors.Errorf("unexpected type %T", input)
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
	event := prEvent(prID, orgID)

	log := zaptest.NewLogger(t)
	db, err := pebble.Open("golovach_lena.db", &pebble.Options{FS: vfs.NewMem()})
	a.NoError(err)
	store := storage.NewMsgID(db)

	a.NoError(store.UpdateLastMsgID(channel.ChannelID, lastMsgID))
	a.NoError(store.SetPRNotification(event, msgID))

	invoker := &mockInvoker{}
	raw := tg.NewClient(invoker)
	sender := message.NewSender(raw).WithResolver(mockResolver{
		"gotd_ru": channel,
	})
	hook := NewWebhook(storage.NewMsgID(db), sender, "secret").WithLogger(log)

	err = hook.handlePRClosed(ctx, event)
	a.NoError(err)

	a.NotNil(invoker.lastReq)
	a.Contains(invoker.lastReq.Message, "opened")
}
