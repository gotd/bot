package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/brpaz/echozap"
	"github.com/cockroachdb/pebble"
	"github.com/google/go-github/v33/github"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/gotd/contrib/invoker"
	"github.com/gotd/contrib/updates"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/docs"
	"github.com/gotd/bot/internal/gentext"
	"github.com/gotd/bot/internal/gh"
	"github.com/gotd/bot/internal/metrics"
	"github.com/gotd/bot/internal/storage"
	bolt "go.etcd.io/bbolt"
)

type App struct {
	client     *telegram.Client
	token      string
	raw        *tg.Client
	sender     *message.Sender
	downloader *downloader.Downloader

	lazy       *LazyHandler
	gaps       *BoltState
	dispatcher tg.UpdateDispatcher

	db      *pebble.DB
	index   *docs.Search
	storage storage.MsgID
	mux     dispatch.MessageMux
	bot     *dispatch.Bot

	gpt2   gentext.Net
	gpt3   gentext.Net
	github *github.Client
	http   *http.Client
	mts    metrics.Metrics
	logger *zap.Logger
}

func InitApp(mts metrics.Metrics, logger *zap.Logger) (_ *App, rerr error) {
	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return nil, xerrors.Errorf("APP_ID not set or invalid %q: %w", os.Getenv("APP_ID"), err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return nil, xerrors.New("no APP_HASH provided")
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, xerrors.New("no BOT_TOKEN provided")
	}

	// Setting up session storage.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, xerrors.Errorf("get home: %w", err)
	}
	sessionDir := filepath.Join(home, ".td")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return nil, xerrors.Errorf("mkdir: %w", err)
	}

	stateDb, err := bolt.Open(filepath.Join(sessionDir, "gaps-state.bbolt"), fs.ModePerm, bolt.DefaultOptions)
	if err != nil {
		return nil, xerrors.Errorf("state database: %w", err)
	}

	db, err := pebble.Open(
		filepath.Join(sessionDir, fmt.Sprintf("bot.%s.state", tokHash(token))),
		&pebble.Options{},
	)
	if err != nil {
		return nil, xerrors.Errorf("database: %w", err)
	}
	defer func() {
		if rerr != nil {
			multierr.AppendInto(&rerr, db.Close())
		}
	}()
	msgIDStore := storage.NewMsgID(db)

	lz := new(LazyHandler)
	client := telegram.NewClient(appID, appHash, telegram.Options{
		Logger: logger.Named("client"),
		SessionStorage: &session.FileStorage{
			Path: filepath.Join(sessionDir, sessionFileName(token)),
		},
		UpdateHandler: dispatch.NewLoggedDispatcher(
			lz, logger.Named("updates"),
		),
	})
	raw := tg.NewClient(invoker.NewUpdateHook(client, lz.Handle))
	sender := message.NewSender(raw)
	dd := downloader.NewDownloader()

	mux := dispatch.NewMessageMux()
	var h dispatch.MessageHandler = metrics.NewMiddleware(mux, dd, mts)
	h = storage.NewHook(h, msgIDStore)

	dispatcher := tg.NewUpdateDispatcher()
	b := dispatch.NewBot(raw).
		WithSender(sender).
		WithLogger(logger).
		Register(dispatcher).
		OnMessage(h)

	httpTransport := http.DefaultTransport
	app := &App{
		client:     client,
		token:      token,
		raw:        raw,
		sender:     sender,
		downloader: dd,
		lazy:       lz,
		gaps:       NewBoltState(stateDb),
		dispatcher: dispatcher,
		db:         db,
		storage:    msgIDStore,
		mux:        mux,
		bot:        b,
		http:       &http.Client{Transport: httpTransport},
		mts:        mts,
		logger:     logger,
	}

	if schemaPath, ok := os.LookupEnv("SCHEMA_PATH"); ok {
		search, err := setupIndex(sessionDir, schemaPath)
		if err != nil {
			return nil, xerrors.Errorf("create search: %w", err)
		}
		app.index = search
		b.OnInline(docs.New(search))
	}

	gpt2, err := setupGPT2(app.http)
	if err != nil {
		return nil, xerrors.Errorf("setup gpt2: %w", err)
	}
	app.gpt2 = gpt2

	gpt3, err := setupGPT3(app.http)
	if err != nil {
		return nil, xerrors.Errorf("setup gpt3: %w", err)
	}
	app.gpt3 = gpt3

	if v, ok := os.LookupEnv("GITHUB_APP_ID"); ok {
		ghClient, err := setupGithub(v, httpTransport)
		if err != nil {
			return nil, xerrors.Errorf("setup github: %w", err)
		}
		app.github = ghClient
	}

	return app, nil
}

func (b *App) Close() error {
	err := b.db.Close()
	if b.index != nil {
		err = multierr.Append(err, b.index.Close())
	}
	return err
}

func (b *App) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	if secret, ok := os.LookupEnv("GITHUB_SECRET"); ok {
		logger := b.logger.Named("webhook")

		httpAddr := os.Getenv("HTTP_ADDR")
		if httpAddr == "" {
			httpAddr = "localhost:8080"
		}

		webhook := gh.NewWebhook(b.storage, b.sender, secret).
			WithLogger(logger)
		if notifyGroup, ok := os.LookupEnv("TG_NOTIFY_GROUP"); ok {
			webhook = webhook.WithNotifyGroup(notifyGroup)
		}

		e := echo.New()
		e.Use(
			middleware.Recover(),
			middleware.RequestID(),
			echozap.ZapLogger(logger.Named("requests")),
		)
		webhook.RegisterRoutes(e)

		server := http.Server{
			Addr:    httpAddr,
			Handler: e,
		}
		group.Go(func() error {
			logger.Info("ListenAndServe", zap.String("addr", server.Addr))
			return server.ListenAndServe()
		})
		group.Go(func() error {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			logger.Info("Shutdown", zap.String("addr", server.Addr))
			if err := server.Shutdown(shutCtx); err != nil {
				return multierr.Append(err, server.Close())
			}
			return nil
		})
	}

	group.Go(func() error {
		return b.client.Run(ctx, func(ctx context.Context) error {
			b.logger.Debug("Client initialized")

			status, err := b.client.AuthStatus(ctx)
			if err != nil {
				return xerrors.Errorf("auth status: %w", err)
			}

			if !status.Authorized {
				if _, err := b.client.AuthBot(ctx, b.token); err != nil {
					return xerrors.Errorf("login: %w", err)
				}

				// Refresh auth status.
				status, err = b.client.AuthStatus(ctx)
				if err != nil {
					return xerrors.Errorf("auth status: %w", err)
				}
			} else {
				b.logger.Info("Bot login restored",
					zap.String("name", status.User.Username),
				)
			}

			if _, disableRegister := os.LookupEnv("DISABLE_COMMAND_REGISTER"); !disableRegister {
				if err := b.mux.RegisterCommands(ctx, b.raw); err != nil {
					return xerrors.Errorf("register commands: %w", err)
				}
			}

			gaps := updates.New(updates.Config{
				RawClient: b.raw,
				Handler:   NewGapAdapter(ctx, b.logger.Named("adapter"), b.dispatcher),
				SelfID:    status.User.ID,
				IsBot:     true,
				Storage:   b.gaps,
				Logger:    b.logger.Named("gaps"),
			})

			b.lazy.Init(telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
				gaps.Handle(u)
				return nil
			}))

			return gaps.Run(ctx)
		})
	})
	return group.Wait()
}

func runBot(ctx context.Context, mts metrics.Metrics, logger *zap.Logger) (rerr error) {
	app, err := InitApp(mts, logger)
	if err != nil {
		return xerrors.Errorf("initialize: %w", err)
	}
	defer func() {
		multierr.AppendInto(&rerr, app.Close())
	}()

	if err := setupBot(app); err != nil {
		return xerrors.Errorf("setup: %w", err)
	}

	if err := app.Run(ctx); err != nil {
		return xerrors.Errorf("run: %w", err)
	}
	return nil
}
