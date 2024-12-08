package tgmanager

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/gotd/bot/internal/ent"
	"github.com/gotd/bot/internal/ent/telegramaccount"
)

type Account struct {
	client *telegram.Client
	number string
	lg     *zap.Logger
	db     *ent.Client
	tracer trace.Tracer
}

// terminalAuth implements auth.UserAuthenticator prompting the terminal for
// input.
type codeAuth struct {
	phone string
	acc   *Account
}

func (codeAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("not implemented")
}

func (codeAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}

func (a codeAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	// Waiting for code.
	return a.acc.WaitForCode(ctx, sentCode)
}

func (a codeAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (codeAuth) Password(_ context.Context) (string, error) {
	return "", errors.New("password not supported")
}

func NewAccount(lg *zap.Logger, db *ent.Client, tracer trace.Tracer, number string) *Account {
	acc := &Account{
		lg:     lg.Named("account"),
		number: number,
		db:     db,
		tracer: tracer,
	}

	// https://github.com/telegramdesktop/tdesktop/blob/dev/docs/api_credentials.md
	client := telegram.NewClient(17349, "344583e45741c457fe1862106095a5eb", telegram.Options{
		DCList: dcs.Test(),
		Logger: lg.Named("client"),
		SessionStorage: &SessionStorage{
			id: number,
			db: db,
		},
	})
	acc.withClient(client)

	return acc
}

func (a *Account) withClient(client *telegram.Client) *Account {
	a.client = client
	return a
}

func (a *Account) setState(ctx context.Context, state telegramaccount.State) error {
	return a.db.TelegramAccount.UpdateOneID(a.number).
		SetState(state).
		Exec(ctx)
}

func (a *Account) Run(ctx context.Context) error {
	if a.client == nil {
		return errors.New("client is not initialized")
	}
	a.lg.Info("Starting")
	flow := auth.NewFlow(&codeAuth{
		phone: a.number,
		acc:   a,
	}, auth.SendCodeOptions{})
	return a.client.Run(ctx, func(ctx context.Context) error {
		a.lg.Info("Running")
		if err := a.client.Auth().IfNecessary(ctx, flow); err != nil {
			return errors.Wrap(err, "auth")
		}
		a.lg.Info("Auth ok")
		if err := a.setState(ctx, telegramaccount.StateActive); err != nil {
			return errors.Wrap(err, "update account")
		}
		<-ctx.Done()
		return ctx.Err()
	})
}

func (a *Account) WaitForCode(ctx context.Context, code *tg.AuthSentCode) (ret string, rerr error) {
	// Wait for code to be sent via API.
	ctx, span := a.tracer.Start(ctx, "WaitForCode")
	defer func() {
		if rerr != nil {
			span.RecordError(rerr)
		}
		span.End()
	}()

	a.lg.Info("Waiting for code")
	if err := a.setState(ctx, telegramaccount.StateCodeSent); err != nil {
		return "", errors.Wrap(err, "update account")
	}

	start := time.Now()
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			acc, err := a.db.TelegramAccount.Get(ctx, a.number)
			if err != nil {
				return "", errors.Wrap(err, "get account")
			}
			if acc.Code == nil || acc.CodeAt == nil || *acc.Code == "" {
				a.lg.Info("Code not received")
				continue
			}
			if acc.CodeAt.Before(start) {
				a.lg.Info("Code expired")
				continue
			}
			return *acc.Code, nil
		}
	}
}
