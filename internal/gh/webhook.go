package gh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/pebble"
	"github.com/google/go-github/v33/github"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/peer"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// Webhook is a Github events web hook handler.
type Webhook struct {
	db *pebble.DB

	sender       *message.Sender
	notifyGroup  string
	githubSecret string

	logger *zap.Logger
}

// NewWebhook creates new web hook handler.
func NewWebhook(db *pebble.DB, sender *message.Sender, githubSecret string) *Webhook {
	return &Webhook{
		db:           db,
		sender:       sender,
		notifyGroup:  "gotd_ru",
		githubSecret: githubSecret,
		logger:       zap.NewNop(),
	}
}

// WithSender sets message sender to use.
func (h *Webhook) WithSender(sender *message.Sender) *Webhook {
	h.sender = sender
	return h
}

// WithNotifyGroup sets channel name to send notifications.
func (h *Webhook) WithNotifyGroup(domain string) *Webhook {
	h.notifyGroup = domain
	return h
}

// WithLogger sets logger to use.
func (h *Webhook) WithLogger(logger *zap.Logger) *Webhook {
	h.logger = logger
	return h
}

// RegisterRoutes registers hook using given Echo router.
func (h Webhook) RegisterRoutes(e *echo.Echo) {
	e.POST("/hook", h.handleHook)
}

func (h Webhook) handleHook(e echo.Context) error {
	payload, err := github.ValidatePayload(e.Request(), []byte(h.githubSecret))
	if err != nil {
		h.logger.Info("Failed to validate payload")
		return echo.ErrNotFound
	}
	whType := github.WebHookType(e.Request())
	if whType == "security_advisory" {
		// Current github library is unable to handle this.
		return e.String(http.StatusOK, "ignored")
	}

	event, err := github.ParseWebHook(whType, payload)
	if err != nil {
		h.logger.Error("Failed to parse webhook", zap.Error(err))
		return echo.ErrInternalServerError
	}

	log := h.logger.With(
		zap.String("type", fmt.Sprintf("%T", event)),
	)
	log.Info("Processing event")
	ctx := e.Request().Context()
	switch event := event.(type) {
	case *github.PullRequestEvent:
		return h.handlePR(ctx, event)
	case *github.ReleaseEvent:
		return h.handleRelease(ctx, event)
	case *github.RepositoryEvent:
		return h.handleRepo(ctx, event)
	default:
		log.Info("No handler")
		return e.String(http.StatusOK, "ok")
	}
}

func (h Webhook) notifyPeer(ctx context.Context) (tg.InputPeerClass, error) {
	p, err := h.sender.ResolveDomain(h.notifyGroup, peer.OnlyChannel).AsInputPeer(ctx)
	if err != nil {
		return nil, xerrors.Errorf("resolve: %w", err)
	}
	return p, nil
}
