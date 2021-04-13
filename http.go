package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/go-github/v33/github"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/clock"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (b *Bot) RegisterRoutes(e *echo.Echo) {
	e.POST("/hook", b.handleHook)
}

func (b *Bot) WithNotifyGroup(s string) *Bot {
	b.notifyGroup = s
	return b
}

func (b *Bot) handleHook(e echo.Context) error {
	payload, err := github.ValidatePayload(e.Request(), []byte(b.githubSecret))
	if err != nil {
		b.logger.Info("Failed to validate payload")
		return echo.ErrNotFound
	}
	whType := github.WebHookType(e.Request())
	if whType == "security_advisory" {
		// Current github library is unable to handle this.
		return e.String(http.StatusOK, "ignored")
	}

	event, err := github.ParseWebHook(whType, payload)
	if err != nil {
		b.logger.Error("Failed to parse webhook", zap.Error(err))
		return echo.ErrInternalServerError
	}

	log := b.logger.With(
		zap.String("type", fmt.Sprintf("%T", event)),
	)
	log.Info("Processing event")
	switch event := event.(type) {
	case *github.PullRequestEvent:
		return b.handlePR(e.Request().Context(), event)
	default:
		log.Info("No handler")
		return e.String(http.StatusOK, "ok")
	}
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
