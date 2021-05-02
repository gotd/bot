package gh

import (
	"context"
	"fmt"

	"github.com/google/go-github/v33/github"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/styling"
)

func (h Webhook) handleRelease(ctx context.Context, e *github.ReleaseEvent) error {
	if e.GetAction() != "published" {
		return nil
	}

	p, err := h.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	if _, err := h.sender.To(p).StyledText(ctx,
		styling.Plain("New release: "),
		styling.TextURL(e.GetRelease().GetTagName(), e.GetRelease().GetHTMLURL()),
		styling.Plain(fmt.Sprintf(" for %s", e.GetRepo().GetFullName())),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}
