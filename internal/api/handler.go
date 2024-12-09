package api

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/zctx"
	"github.com/google/go-github/v42/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/gotd/bot/internal/oas"
	"github.com/gotd/bot/internal/tgmanager"
)

func NewHandler(manager *tgmanager.Manager) *Handler {
	return &Handler{manager: manager}
}

type Handler struct {
	manager *tgmanager.Manager
}

func (h Handler) AcquireTelegramAccount(ctx context.Context, req *oas.AcquireTelegramAccountReq) (*oas.AcquireTelegramAccountOK, error) {
	client, ok := ctx.Value(ghClient{}).(*github.Client)
	if !ok {
		return nil, errors.New("github client not found")
	}
	if req.RepoOwner != "gotd" {
		return nil, errors.New("unsupported repo owner")
	}
	repo, _, err := client.Repositories.Get(ctx, req.RepoOwner, req.RepoName)
	if err != nil {
		return nil, errors.Wrap(err, "get repo")
	}
	job, _, err := client.Actions.GetWorkflowJobByID(ctx, req.RepoOwner, req.RepoName, int64(req.JobID))
	if err != nil {
		return nil, errors.Wrap(err, "get job")
	}
	zctx.From(ctx).Info("AcquireTelegramAccount",
		zap.String("repo", repo.GetFullName()),
		zap.String("job", job.GetName()),
	)

	lease, err := h.manager.Acquire()
	if err != nil {
		return nil, errors.Wrap(err, "acquire")
	}

	return &oas.AcquireTelegramAccountOK{
		AccountID: oas.TelegramAccountID(lease.Account),
		Token:     lease.Token,
	}, nil
}

type ghClient struct{}

func (h Handler) HandleTokenAuth(ctx context.Context, operationName oas.OperationName, t oas.TokenAuth) (context.Context, error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: t.APIKey},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	ctx = context.WithValue(ctx, ghClient{}, client)
	return ctx, nil
}

func (h Handler) GetHealth(ctx context.Context) (*oas.Health, error) {
	return &oas.Health{
		Status: "ok",
	}, nil
}

func (h Handler) HeartbeatTelegramAccount(ctx context.Context, params oas.HeartbeatTelegramAccountParams) error {
	if params.Forget.Value {
		h.manager.Forget(params.Token)
		return nil
	}
	return h.manager.Heartbeat(params.Token)
}

func (h Handler) ReceiveTelegramCode(ctx context.Context, params oas.ReceiveTelegramCodeParams) (*oas.ReceiveTelegramCodeOK, error) {
	code, err := h.manager.LeaseCode(ctx, params.Token)
	if err != nil {
		return nil, errors.Wrap(err, "lease code")
	}
	var rc oas.OptString
	if code != "" {
		rc.SetTo(code)
	}
	return &oas.ReceiveTelegramCodeOK{Code: rc}, nil
}

func (h Handler) NewError(ctx context.Context, err error) *oas.ErrorStatusCode {
	return &oas.ErrorStatusCode{
		StatusCode: 500,
		Response: oas.Error{
			ErrorMessage: err.Error(),
		},
	}
}

var _ oas.Handler = &Handler{}
var _ oas.SecurityHandler = &Handler{}
