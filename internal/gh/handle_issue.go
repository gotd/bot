package gh

import (
	"context"
	"fmt"

	"github.com/google/go-github/v33/github"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/entity"
	"github.com/gotd/td/telegram/message/styling"
)

func (h Webhook) handleIssue(ctx context.Context, e *github.IssuesEvent) error {
	if e.GetAction() != "opened" {
		return nil
	}

	p, err := h.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	issue := e.GetIssue()
	sender := e.GetSender()
	formatter := func(eb *entity.Builder) error {
		eb.Plain("New issue ")
		urlName := fmt.Sprintf("%s#%d",
			e.GetRepo().GetFullName(),
			issue.GetNumber(),
		)
		eb.TextURL(urlName, issue.GetHTMLURL())
		eb.Plain(" by ")
		eb.TextURL(sender.GetLogin(), sender.GetHTMLURL())
		eb.Plain("\n\n")

		eb.Italic(issue.GetTitle())
		eb.Plain("\n\n")

		length := len(issue.Labels)
		if length > 0 {
			eb.Italic("Labels: ")

			for idx, label := range issue.Labels {
				switch label.GetName() {
				case "":
					continue
				case "bug":
					eb.Bold(label.GetName())
				default:
					eb.Italic(label.GetName())
				}

				if idx != length-1 {
					eb.Plain(", ")
				}
			}
		}
		return nil
	}

	if _, err := h.sender.To(p).NoWebpage().
		StyledText(ctx, styling.Custom(formatter)); err != nil {
		return xerrors.Errorf("send: %w", err)
	}
	return nil
}
