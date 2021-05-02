package gh

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/google/go-github/v33/github"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
)

const (
	githubOwner    = "gotd"
	githubRepo     = "td"
	githubRef      = "main"
	githubWorkflow = "bot.yml"
)

// Handler implements GitHub request handler.
type Handler struct {
	github *github.Client
}

// New creates new Handler.
func New(client *github.Client) Handler {
	return Handler{github: client}
}

// OnMessage implements bot.MessageHandler.
func (h Handler) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	return e.WithReply(ctx, func(reply *tg.Message) error {
		gh := h.github
		if gh == nil {
			if _, err := e.Reply().Text(ctx, "Github integration disabled"); err != nil {
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
			"message":  e.Message,
			"reply_to": reply,
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
		if _, err := e.Reply().StyledText(ctx, styling.Pre(resp.Status)); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	})
}
