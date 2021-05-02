package gh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v33/github"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

// Webhook is a Github events web hook handler.
type Webhook struct {
	sender       *message.Sender
	resolver     peer.Resolver
	notifyGroup  string
	githubSecret string

	logger *zap.Logger
}

// NewWebhook creates new web hook handler.
func NewWebhook(raw *tg.Client, githubSecret string) *Webhook {
	resolver := peer.DefaultResolver(raw)
	return &Webhook{
		sender:       message.NewSender(raw).WithResolver(resolver),
		resolver:     resolver,
		notifyGroup:  "gotd_ru",
		githubSecret: githubSecret,
		logger:       zap.NewNop(),
	}
}

// WithSender sets message sender to use.
func (b *Webhook) WithSender(sender *message.Sender) *Webhook {
	b.sender = sender
	return b
}

// WithNotifyGroup sets channel name to send notifications.
func (b *Webhook) WithNotifyGroup(domain string) *Webhook {
	b.notifyGroup = domain
	return b
}

// WithLogger sets logger to use.
func (b *Webhook) WithLogger(logger *zap.Logger) *Webhook {
	b.logger = logger
	return b
}

// WithResolver sets peer resolver to use.
func (b *Webhook) WithResolver(resolver peer.Resolver) *Webhook {
	b.resolver = resolver
	return b
}

// RegisterRoutes registers hook using given Echo router.
func (b Webhook) RegisterRoutes(e *echo.Echo) {
	e.POST("/hook", b.handleHook)
}

func (b Webhook) handleHook(e echo.Context) error {
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
	ctx := e.Request().Context()
	switch event := event.(type) {
	case *github.PullRequestEvent:
		return b.handlePR(ctx, event)
	case *github.ReleaseEvent:
		return b.handleRelease(ctx, event)
	case *github.RepositoryEvent:
		return b.handleRepo(ctx, event)
	default:
		log.Info("No handler")
		return e.String(http.StatusOK, "ok")
	}
}

func getPullRequestURL(e *github.PullRequestEvent) styling.StyledTextOption {
	urlName := fmt.Sprintf("%s#%d",
		e.GetRepo().GetFullName(),
		e.PullRequest.GetNumber(),
	)

	return styling.TextURL(urlName, e.GetPullRequest().GetHTMLURL())
}

func (b Webhook) handlePRClosed(ctx context.Context, e *github.PullRequestEvent) error {
	if !e.GetPullRequest().GetMerged() {
		b.logger.Info("Ignoring non-merged PR")
		return nil
	}

	p, err := b.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	if _, err := b.sender.To(p).NoWebpage().StyledText(ctx,
		styling.Plain("Pull request "),
		getPullRequestURL(e),
		styling.Plain(" merged:\n\n"),
		styling.Italic(e.GetPullRequest().GetTitle()),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b Webhook) handlePROpened(ctx context.Context, e *github.PullRequestEvent) error {
	p, err := b.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
	}

	if _, err := b.sender.To(p).NoWebpage().StyledText(ctx,
		styling.Plain("New pull request "),
		getPullRequestURL(e),
		styling.Plain(" opened:\n\n"),
		styling.Italic(e.GetPullRequest().GetTitle()),
	); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b Webhook) handlePR(ctx context.Context, e *github.PullRequestEvent) error {
	// Ignore PR-s from dependabot (too much noise).
	// TODO(ernado): delay and merge into single message
	if e.GetPullRequest().GetUser().GetLogin() == "dependabot[bot]" {
		b.logger.Info("Ignored PR from dependabot")
		return nil
	}

	switch e.GetAction() {
	case "opened":
		return b.handlePROpened(ctx, e)
	case "closed":
		return b.handlePRClosed(ctx, e)
	}
	return nil
}

func (b Webhook) handleRelease(ctx context.Context, e *github.ReleaseEvent) error {
	if e.GetAction() != "published" {
		return nil
	}

	p, err := b.notifyPeer(ctx)
	if err != nil {
		return xerrors.Errorf("peer: %w", err)
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

func (b Webhook) handleRepo(ctx context.Context, e *github.RepositoryEvent) error {
	switch e.GetAction() {
	case "created", "publicized":
		p, err := b.notifyPeer(ctx)
		if err != nil {
			return xerrors.Errorf("peer: %w", err)
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

func (b Webhook) notifyPeer(ctx context.Context) (tg.InputPeerClass, error) {
	p, err := b.resolver.ResolveDomain(ctx, b.notifyGroup)
	if err != nil {
		return nil, xerrors.Errorf("resolve: %w", err)
	}
	return p, nil
}
