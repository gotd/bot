package gh

import (
	"context"

	"github.com/google/go-github/v33/github"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/styling"
)

func (h Webhook) handleRepo(ctx context.Context, e *github.RepositoryEvent) error {
	switch e.GetAction() {
	case "created", "publicized":
		p, err := h.notifyPeer(ctx)
		if err != nil {
			return xerrors.Errorf("peer: %w", err)
		}

		if _, err := h.sender.To(p).StyledText(ctx,
			styling.Plain("New repository "),
			styling.TextURL(e.GetRepo().GetFullName(), e.GetRepo().GetHTMLURL()),
		); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	default:
		h.logger.Info("Action ignored", zap.String("action", e.GetAction()))

		return nil
	}
}
