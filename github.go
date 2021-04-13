package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/go-github/v33/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/gotd/td/clock"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

const (
	githubOwner    = "gotd"
	githubRepo     = "td"
	githubRef      = "main"
	githubWorkflow = "bot.yml"
)

func (b *Bot) answerGH(
	ctx context.Context,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
) error {
	return b.getReply(ctx, send, peer, m, func(msg *tg.Message) error {
		gh := b.github
		if gh == nil {
			if _, err := send.Text(ctx, "Github integration disabled"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}

			return nil
		}

		// Create client with short-lived repository installation token.
		inst, _, err := gh.Apps.FindRepositoryInstallation(ctx, githubOwner, githubRepo)
		if err != nil {
			return xerrors.Errorf("find repository installation: %w", err)
		}
		tok, _, err := gh.Apps.CreateInstallationToken(ctx, inst.GetID(), nil)
		if err != nil {
			return xerrors.Errorf("create installation token: %w", err)
		}
		gh = github.NewClient(
			oauth2.NewClient(ctx,
				oauth2.StaticTokenSource(&oauth2.Token{
					AccessToken: tok.GetToken(),
				}),
			),
		)

		// Dispatch workflow. Note that inputs must be strings.
		data, err := json.Marshal(map[string]interface{}{
			"message":  m,
			"reply_to": msg,
		})
		if err != nil {
			return xerrors.Errorf("encode payload: %w", err)
		}
		resp, err := gh.Actions.CreateWorkflowDispatchEventByFileName(ctx,
			githubOwner, githubRepo, githubWorkflow,
			github.CreateWorkflowDispatchEventRequest{
				Ref: githubRef,
				Inputs: map[string]interface{}{
					"telegram": base64.StdEncoding.EncodeToString(data),
				},
			},
		)
		if err != nil {
			return xerrors.Errorf("dispatch workflow: %w", err)
		}

		// Reply with response status.
		if _, err := send.StyledText(ctx, styling.Pre(resp.Status)); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	})
}

type resolved struct {
	time time.Time
	peer tg.InputPeerClass
}

type ttlResolver struct {
	clock    clock.Clock
	resolver peer.Resolver
	duration time.Duration
	mux      sync.Mutex
	data     map[string]resolved
}

func (r *ttlResolver) ResolvePhone(ctx context.Context, phone string) (tg.InputPeerClass, error) {
	return r.resolver.ResolvePhone(ctx, phone)
}

func (r *ttlResolver) ResolveDomain(ctx context.Context, s string) (tg.InputPeerClass, error) {
	r.mux.Lock()
	defer r.mux.Unlock()

	x, ok := r.data[s]
	if ok && r.clock.Now().Sub(x.time) < r.duration {
		return x.peer, nil
	}

	p, err := r.resolver.ResolveDomain(ctx, s)
	if err != nil {
		return nil, err
	}

	x.peer = p
	x.time = r.clock.Now()
	r.data[s] = x

	return p, nil
}

func getPullRequestURL(e *github.PullRequestEvent) styling.StyledTextOption {
	urlName := fmt.Sprintf("%s#%d",
		e.GetRepo().GetFullName(),
		e.PullRequest.GetNumber(),
	)

	return styling.TextURL(urlName, e.GetPullRequest().GetHTMLURL())
}

func (b *Bot) handlePRClosed(ctx context.Context, e *github.PullRequestEvent) error {
	if !e.GetPullRequest().GetMerged() {
		b.logger.Info("Ignoring non-merged PR")
		return nil
	}

	p, err := b.resolver.ResolveDomain(ctx, b.notifyGroup)
	if err != nil {
		return xerrors.Errorf("resolve: %w", err)
	}

	if _, err := b.sender.To(p).StyledText(ctx,
		styling.Plain("Pull request "),
		getPullRequestURL(e),
		styling.Plain(" merged:\n\n"),
		styling.Italic(e.GetPullRequest().GetTitle()),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b *Bot) handlePROpened(ctx context.Context, e *github.PullRequestEvent) error {
	p, err := b.resolver.ResolveDomain(ctx, b.notifyGroup)
	if err != nil {
		return xerrors.Errorf("resolve: %w", err)
	}

	if _, err := b.sender.To(p).StyledText(ctx,
		styling.Plain("New pull request "),
		getPullRequestURL(e),
		styling.Plain(" opened:\n\n"),
		styling.Italic(e.GetPullRequest().GetTitle()),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b *Bot) handlePR(ctx context.Context, e *github.PullRequestEvent) error {
	switch e.GetAction() {
	case "opened":
		return b.handlePROpened(ctx, e)
	case "closed":
		return b.handlePRClosed(ctx, e)
	}
	return nil
}

func (b *Bot) handleRelease(ctx context.Context, e *github.ReleaseEvent) error {
	if e.GetAction() != "published" {
		return nil
	}

	p, err := b.resolver.ResolveDomain(ctx, b.notifyGroup)
	if err != nil {
		return xerrors.Errorf("resolve: %w", err)
	}

	if _, err := b.sender.To(p).StyledText(ctx,
		styling.Plain("New release: "),
		styling.TextURL(e.GetRelease().GetTagName(), e.GetRelease().GetHTMLURL()),
		styling.Plain(fmt.Sprintf(" for %s", e.GetRepo().GetFullName())),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b *Bot) handleRepo(ctx context.Context, e *github.RepositoryEvent) error {
	switch e.GetAction() {
	case "created", "publicized":
		p, err := b.resolver.ResolveDomain(ctx, b.notifyGroup)
		if err != nil {
			return xerrors.Errorf("resolve: %w", err)
		}

		if _, err := b.sender.To(p).StyledText(ctx,
			styling.Plain("New repository "),
			styling.TextURL(e.GetRepo().GetFullName(), e.GetRepo().GetHTMLURL()),
		); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	default:
		b.logger.Info("Action ignored", zap.String("action", e.GetAction()))

		return nil
	}
}
