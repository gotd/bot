package main

import (
	"context"
	"fmt"
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

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/gentext"
	"github.com/gotd/bot/internal/gh"
	"github.com/gotd/bot/internal/gpt"
	"github.com/gotd/bot/internal/inspect"
	"github.com/gotd/bot/internal/metrics"
	"github.com/gotd/bot/internal/storage"
	"github.com/gotd/bot/internal/tits"
)

type App struct {
	client     *telegram.Client
	token      string
	raw        *tg.Client
	resolver   peer.Resolver
	sender     *message.Sender
	downloader *downloader.Downloader

	db  *pebble.DB
	mux dispatch.MessageMux
	bot *dispatch.Bot

	gpt2          gentext.Net
	gpt3          gentext.Net
	github        *github.Client
	httpTransport http.RoundTripper
	http          *http.Client
	mts           metrics.Metrics
	logger        *zap.Logger

	tasks []func(ctx context.Context) error
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

	dispatcher := tg.NewUpdateDispatcher()
	client := telegram.NewClient(appID, appHash, telegram.Options{
		Logger: logger.Named("client"),
		SessionStorage: &session.FileStorage{
			Path: filepath.Join(sessionDir, sessionFileName(token)),
		},
		UpdateHandler: loggedDispatcher{
			log:           logger.Named("updates"),
			UpdateHandler: dispatcher,
		},
	})
	raw := tg.NewClient(client)
	resolver := peer.DefaultResolver(raw)
	sender := message.NewSender(raw).WithResolver(resolver)
	dd := downloader.NewDownloader()

	mux := dispatch.NewMessageMux()
	var h dispatch.MessageHandler = metrics.NewMiddleware(mux, dd, mts)
	h = storage.NewHook(h, db)

	b := dispatch.NewBot(raw, h).
		WithSender(sender).
		WithLogger(logger).
		Register(dispatcher)

	httpTransport := http.DefaultTransport
	app := &App{
		client:        client,
		token:         token,
		raw:           raw,
		resolver:      resolver,
		sender:        sender,
		downloader:    dd,
		db:            db,
		mux:           mux,
		bot:           b,
		httpTransport: httpTransport,
		http:          &http.Client{Transport: httpTransport},
		mts:           mts,
		logger:        logger,
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

func (b *App) AddTask(task func(ctx context.Context) error) {
	b.tasks = append(b.tasks, task)
}

func (b *App) Close() error {
	return b.db.Close()
}

func (b *App) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
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

			return telegram.RunUntilCanceled(ctx, b.client)
		})
	})

	for _, task := range b.tasks {
		group.Go(func() error {
			return task(ctx)
		})
	}

	return group.Wait()
}

func setupBot(app *App) error {
	app.mux.HandleFunc("/bot", "Ping bot", func(ctx context.Context, e dispatch.MessageEvent) error {
		_, err := e.Reply().Text(ctx, "What?")
		return err
	})
	app.mux.HandleFunc("/dice", "Send dice",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Dice(ctx)
			return err
		})
	app.mux.HandleFunc("/darts", "Send darts",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Darts(ctx)
			return err
		})
	app.mux.HandleFunc("/basketball", "Send basketball",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Basketball(ctx)
			return err
		})
	app.mux.HandleFunc("/football", "Send football",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Football(ctx)
			return err
		})
	app.mux.HandleFunc("/casino", "Send casino",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Casino(ctx)
			return err
		})
	app.mux.HandleFunc("/bowling", "Send bowling",
		func(ctx context.Context, e dispatch.MessageEvent) error {
			_, err := e.Reply().Bowling(ctx)
			return err
		})

	app.mux.Handle("/pp", "Pretty print replied message", inspect.Pretty())
	app.mux.Handle("/json", "Print JSON of replied message", inspect.JSON())
	app.mux.Handle("/stat", "Metrics and version", metrics.NewHandler(app.mts))
	app.mux.Handle("/tts", "Text to speech", tits.New(app.http))
	app.mux.Handle("/gpt2", "Complete text with GPT2",
		gpt.New(gentext.NewGPT2().WithClient(app.http)))
	app.mux.Handle("/gpt3", "Complete text with GPT3",
		gpt.New(gentext.NewGPT3().WithClient(app.http)))

	if app.github != nil {
		app.mux.Handle("github", "", gh.New(app.github))
	}
	if secret, ok := os.LookupEnv("GITHUB_SECRET"); ok {
		logger := app.logger.Named("webhook")

		httpAddr := os.Getenv("HTTP_ADDR")
		if httpAddr == "" {
			httpAddr = "localhost:8080"
		}

		webhook := gh.NewWebhook(app.db, app.sender, secret).
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
		app.AddTask(func(ctx context.Context) error {
			return server.ListenAndServe()
		})
		app.AddTask(func(ctx context.Context) error {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			if err := server.Shutdown(shutCtx); err != nil {
				return multierr.Append(err, server.Close())
			}
			return nil
		})
	}

	return nil
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
