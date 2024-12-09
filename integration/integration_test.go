package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-faster/errors"
	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/gotd/bot/internal/oas"
)

type securitySource struct{}

func (s securitySource) TokenAuth(ctx context.Context, operationName oas.OperationName) (oas.TokenAuth, error) {
	return oas.TokenAuth{
		APIKey: os.Getenv("GITHUB_TOKEN"),
	}, nil
}

// terminalAuth implements auth.UserAuthenticator prompting the terminal for
// input.
type codeAuth struct {
	phone  string
	token  uuid.UUID
	client *oas.Client
}

func (codeAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("not implemented")
}

func (codeAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}

func (a codeAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = time.Minute
	bo.MaxInterval = time.Second

	return backoff.RetryWithData(func() (string, error) {
		res, err := a.client.ReceiveTelegramCode(ctx, oas.ReceiveTelegramCodeParams{
			Token: a.token,
		})
		if err != nil {
			return "", err
		}
		if res.Code.Value == "" {
			return "", errors.New("no code")
		}
		return res.Code.Value, err
	}, bo)
}

func (a codeAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (codeAuth) Password(_ context.Context) (string, error) {
	return "", errors.New("password not supported")
}

func TestIntegration(t *testing.T) {
	// Integration tests should be explicitly enabled,
	// also should be in GitHub actions with token.
	if _, ok := os.LookupEnv("GITHUB_TOKEN"); !ok {
		t.Skip("no token")
	}
	if ok, _ := strconv.ParseBool(os.Getenv("E2E")); !ok {
		t.Skip("E2E=1 not set")
	}

	jobID, err := strconv.Atoi(os.Getenv("GITHUB_JOB_ID"))
	require.NoError(t, err)

	ctx := context.Background()
	client, err := oas.NewClient("https://bot.gotd.dev", securitySource{})
	require.NoError(t, err)

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = time.Minute
	bo.MaxInterval = time.Second

	res, err := backoff.RetryNotifyWithData(func() (*oas.AcquireTelegramAccountOK, error) {
		return client.AcquireTelegramAccount(ctx, &oas.AcquireTelegramAccountReq{
			RepoOwner: "gotd",
			RepoName:  "bot",
			JobID:     jobID,
		})
	}, bo, func(err error, duration time.Duration) {
		t.Logf("Error: %v, retrying in %v", err, duration)
	})
	require.NoError(t, err)

	lg := zaptest.NewLogger(t)
	au := codeAuth{
		phone: string(res.AccountID),
		token: res.Token,
	}
	tgc := telegram.NewClient(17349, "344583e45741c457fe1862106095a5eb", telegram.Options{
		DCList: dcs.Test(),
		Logger: lg.Named("client"),
	})
	require.NoError(t, tgc.Run(ctx, func(ctx context.Context) error {
		t.Log("Auth")
		if err := tgc.Auth().IfNecessary(ctx, auth.NewFlow(au, auth.SendCodeOptions{})); err != nil {
			return errors.Wrap(err, "auth")
		}
		t.Log("Auth ok")
		return nil
	}))
}
