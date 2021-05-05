package gh

import (
	"context"
	"fmt"
	"net/url"
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
	if u, _ := url.Parse(e.GetPullRequest().GetHTMLURL()); u != nil {
		files, checks := *u, *u
		files.Path = path.Join(files.Path, "files")
		checks.Path = path.Join(checks.Path, "checks")
		r = r.Row(
			markup.URL("Diff🔀", files.String()),
			markup.URL("Checks▶", checks.String()),
		)
	}
	return r
}

func (h Webhook) handlePRClosed(ctx context.Context, event *github.PullRequestEvent) (rerr error) {
	prID := event.GetPullRequest().GetNumber()
	log := h.logger.With(zap.Int("pr", prID), zap.String("repo", event.GetRepo().GetFullName()))
	if !event.GetPullRequest().GetMerged() {
		h.logger.Info("Ignoring non-merged PR")
		return nil
	}

	p, err := h.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	var replyID int
	fallback := func(ctx context.Context) error {
		r := h.notify(p, event)
		if replyID != 0 {
			r = r.Reply(replyID)
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

	msgID, lastMsgID, err := h.storage.FindPRNotification(ch.ChannelID, event)
	if msgID != 0 {
		log.Debug("Found PR notification ID", zap.Int("msg_id", msgID))
		replyID = msgID
	}
	if err != nil {
		if xerrors.Is(err, pebble.ErrNotFound) {
			return fallback(ctx)
		}
		return xerrors.Errorf("find notification: %w", err)
	}

	log.Debug("Found last message ID", zap.Int("msg_id", lastMsgID), zap.Int("channel", ch.ChannelID))
	if lastMsgID-msgID > 10 {
		log.Debug("Can't merge, send new message")
		return fallback(ctx)
	}

	if _, err := h.notify(p, event).Edit(msgID).StyledText(ctx,
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

	ch, ok := p.(*tg.InputPeerChannel)
	if !ok {
		return h.storage.SetPRNotification(event, msgID)
	}

	return multierr.Append(
		h.storage.UpdateLastMsgID(ch.ChannelID, msgID),
		h.storage.SetPRNotification(event, msgID),
	)
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