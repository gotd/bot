package main

import (
	"fmt"
	"net/http"

	"github.com/google/go-github/v33/github"
	"go.uber.org/zap"
)

func (b *Bot) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/hook", b.handleHook)
}

func (b *Bot) handleHook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(b.githubSecret))
	if err != nil {
		b.logger.Info("Failed to validate payload")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		b.logger.Error("Failed to parse webhook", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	b.logger.Info("Got webhook", zap.String("type", fmt.Sprintf("%T", event)))

	w.WriteHeader(http.StatusOK)
}
