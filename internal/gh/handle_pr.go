package gh

import (
	"context"
	"fmt"
	"path"

	"github.com/cockroachdb/pebble"
	"github.com/google/go-github/v33/github"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func getPullRequestURL(e *github.PullRequestEvent) styling.StyledTextOption {
	urlName := fmt.Sprintf("%s#%d",
		e.GetRepo().GetFullName(),
		e.PullRequest.GetNumber(),
	)

	return styling.TextURL(urlName, e.GetPullRequest().GetHTMLURL())
}

func (h Webhook) notify(p tg.InputPeerClass, e *github.PullRequestEvent) *message.Builder {
	r := h.sender.To(p).NoWebpage()
	if u := e.PullRequest.GetHTMLURL(); u != "" {
		r = r.Row(markup.URL("Diff🔀", path.Join(u, "files")))
		r = r.Row(markup.URL("Checks▶", path.Join(u, "checks")))
	}
	return r
}

func (h Webhook) handlePRClosed(ctx context.Context, event *github.PullRequestEvent) (rerr error) {
	prID := event.GetPullRequest().GetNumber()
	log := h.logger.With(zap.Int("pr", prID), zap.String("repo", event.Repo.GetName()))
	if !event.GetPullRequest().GetMerged() {
		h.logger.Info("Ignoring non-merged PR")
		return nil
	}

	p, err := h.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	var replyID int64
	fallback := func(ctx context.Context) error {
		r := h.notify(p, event)
		if replyID != 0 {
			r = r.Reply(int(replyID))
		}
		if _, err := r.StyledText(ctx,
			styling.Plain("Pull request "),
			getPullRequestURL(event),
			styling.Plain(" merged:\n\n"),
			styling.Italic(event.GetPullRequest().GetTitle()),
		); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	}

	ch, ok := p.(*tg.InputPeerChannel)
	if !ok {
		return fallback(ctx)
	}

	snap := h.db.NewSnapshot()
	defer func() {
		multierr.AppendInto(&rerr, snap.Close())
	}()

	msgID, err := findID(snap, event)
	if err != nil {
		if xerrors.Is(err, pebble.ErrNotFound) {
			return fallback(ctx)
		}
		return xerrors.Errorf("find msg ID of PR #%d notification", prID)
	}
	log.Debug("Found PR notification ID", zap.Int64("msg_id", msgID))
	replyID = msgID

	lastMsgID, err := findLastMsgID(snap, ch.ChannelID)
	if err != nil {
		if xerrors.Is(err, pebble.ErrNotFound) {
			return fallback(ctx)
		}
		return xerrors.Errorf("find last msg ID of channel %d", ch.ChannelID)
	}
	log.Debug("Found last message ID", zap.Int64("msg_id", lastMsgID), zap.Int("channel", ch.ChannelID))

	if lastMsgID-msgID > 10 {
		log.Debug("Can't merge, send new message")
		return fallback(ctx)
	}

	if _, err := h.notify(p, event).Edit(int(msgID)).StyledText(ctx,
		styling.Plain("Pull request "),
		getPullRequestURL(event),
		styling.Strike(" opened"),
		styling.Plain(" merged:\n\n"),
		styling.Italic(event.GetPullRequest().GetTitle()),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (h Webhook) handlePROpened(ctx context.Context, event *github.PullRequestEvent) error {
	p, err := h.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}
	action := " opened"
	if event.GetPullRequest().GetDraft() {
		action = " drafted"
	}

	msgID, err := unpack.MessageID(h.notify(p, event).StyledText(ctx,
		styling.Plain("New pull request "),
		getPullRequestURL(event),
		styling.Plain(action),
		styling.Plain(" by "),
		styling.TextURL(event.Sender.GetLogin(), event.Sender.GetHTMLURL()),
		styling.Plain(":\n\n"),
		styling.Italic(event.GetPullRequest().GetTitle()),
	))
	if err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return addID(h.db, event, msgID)
}

func (h Webhook) handlePR(ctx context.Context, e *github.PullRequestEvent) error {
	// Ignore PR-s from dependabot (too much noise).
	// TODO(ernado): delay and merge into single message
	if e.GetPullRequest().GetUser().GetLogin() == "dependabot[bot]" {
		h.logger.Info("Ignored PR from dependabot")
		return nil
	}

	switch e.GetAction() {
	case "opened":
		return h.handlePROpened(ctx, e)
	case "closed":
		return h.handlePRClosed(ctx, e)
	}
	return nil
}
