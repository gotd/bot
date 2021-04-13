package main

import (
	"fmt"
	"net/http"

	"github.com/google/go-github/v33/github"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
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
	ctx := e.Request().Context()
	switch event := event.(type) {
	case *github.PullRequestEvent:
		return b.handlePR(ctx, event)
	case *github.ReleaseEvent:
		return b.handleRelease(ctx, event)
	default:
		log.Info("No handler")
		return e.String(http.StatusOK, "ok")
	}
}
