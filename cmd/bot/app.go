package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/brpaz/echozap"
	"github.com/cockroachdb/pebble"
	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/app"
	"github.com/go-faster/sdk/zctx"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-github/v42/github"
	"github.com/gotd/contrib/oteltg"
	tgredis "github.com/gotd/contrib/redis"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/gotd/bot/internal/api"
	iapp "github.com/gotd/bot/internal/app"
	"github.com/gotd/bot/internal/botapi"
	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/docs"
	"github.com/gotd/bot/internal/entdb"
	"github.com/gotd/bot/internal/oas"
	"github.com/gotd/bot/internal/storage"
	"github.com/gotd/bot/internal/tgmanager"
)

type App struct {
	client *telegram.Client
	token  string
	raw    *tg.Client
	sender *message.Sender

	dispatcher tg.UpdateDispatcher

	db      *pebble.DB
	index   *docs.Search
	storage storage.MsgID
	mux     dispatch.MessageMux
	bot     *dispatch.Bot

	github  *github.Client
	http    *http.Client
	logger  *zap.Logger
	cache   *redis.Client
	manager *tgmanager.Manager
	srv     *oas.Server
}

func InitApp(m *app.Metrics, mm *iapp.Metrics, logger *zap.Logger) (_ *App, rerr error) {
	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return nil, errors.Wrapf(err, "APP_ID not set or invalid %q", os.Getenv("APP_ID"))
	}

	mw, err := oteltg.New(m.MeterProvider(), m.TracerProvider())
	if err != nil {
		return nil, errors.Wrap(err, "oteltg")
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return nil, errors.New("no APP_HASH provided")
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, errors.New("no BOT_TOKEN provided")
	}

	r := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	// Setting up session storage.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "get home")
	}
	sessionDir := filepath.Join(home, ".td")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return nil, errors.Wrap(err, "mkdir")
	}

	db, err := pebble.Open(
		filepath.Join(sessionDir, fmt.Sprintf("bot.%s.state", tokHash(token))),
		&pebble.Options{},
	)
	if err != nil {
		return nil, errors.Wrap(err, "database")
	}
	defer func() {
		if rerr != nil {
			multierr.AppendInto(&rerr, db.Close())
		}
	}()
	msgIDStore := storage.NewMsgID(db)

	dispatcher := tg.NewUpdateDispatcher()
	client := telegram.NewClient(appID, appHash, telegram.Options{
		Logger:         logger.Named("client"),
		SessionStorage: tgredis.NewSessionStorage(r, "gotd_bot_session"),
		UpdateHandler:  dispatcher,
		Middlewares: []telegram.Middleware{
			telegram.MiddlewareFunc(func(next tg.Invoker) telegram.InvokeFunc {
				return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
					return next.Invoke(zctx.Base(ctx, logger), input, output)
				}
			}),
			mw,
		},
	})
	raw := client.API()
	sender := message.NewSender(raw)
	dd := downloader.NewDownloader()
	httpTransport := http.DefaultTransport
	httpClient := &http.Client{
		Transport: httpTransport,
		Timeout:   15 * time.Second,
	}

	mux := dispatch.NewMessageMux()
	var h dispatch.MessageHandler = iapp.NewMiddleware(mux, dd, mm, iapp.MiddlewareOptions{
		BotAPI: botapi.NewClient(token, botapi.Options{
			HTTPClient: httpClient,
		}),
		Logger: logger.Named("metrics"),
	})
	h = storage.NewHook(h, msgIDStore)

	b := dispatch.NewBot(raw).
		WithSender(sender).
		WithLogger(logger).
		Register(dispatcher).
		OnMessage(h)

	edb, err := entdb.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, errors.Wrap(err, "open database")
	}
	manager, err := tgmanager.NewManager(logger.Named("tgmanager"), edb, m.MeterProvider(), m.TracerProvider())
	if err != nil {
		return nil, errors.Wrap(err, "manager")
	}
	handler := api.NewHandler(manager)
	srv, err := oas.NewServer(handler, handler,
		oas.WithTracerProvider(m.TracerProvider()),
		oas.WithMeterProvider(m.MeterProvider()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "oas")
	}

	a := &App{
		manager:    manager,
		client:     client,
		token:      token,
		raw:        raw,
		sender:     sender,
		dispatcher: dispatcher,
		db:         db,
		storage:    msgIDStore,
		mux:        mux,
		bot:        b,
		http:       httpClient,
		logger:     logger,
		cache:      r,
		srv:        srv,
	}

	if schemaPath, ok := os.LookupEnv("SCHEMA_PATH"); ok {
		search, err := setupIndex(sessionDir, schemaPath)
		if err != nil {
			return nil, errors.Wrap(err, "create search")
		}
		a.index = search
		b.OnInline(docs.New(search))
	}

	if v, ok := os.LookupEnv("GITHUB_APP_ID"); ok {
		ghClient, err := setupGithub(v, httpTransport)
		if err != nil {
			return nil, errors.Wrap(err, "setup github")
		}
		a.github = ghClient
	}

	return a, nil
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
	group.Go(func() error {
		return b.manager.Run(ctx)
	})

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = "localhost:8080"
	}

	logger := b.logger.Named("http")
	e := echo.New()
	e.Use(
		middleware.Recover(),
		middleware.RequestID(),
		echozap.ZapLogger(logger.Named("requests")),
	)

	e.GET("/probe/startup", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/probe/ready", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/status", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	mux := http.NewServeMux()
	mux.Handle("/", e)
	mux.Handle("/api/", b.srv)

	server := http.Server{
		Addr:    httpAddr,
		Handler: mux,
		BaseContext: func(listener net.Listener) context.Context {
			return zctx.Base(ctx, b.logger)
		},
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

	group.Go(func() error {
		return b.client.Run(ctx, func(ctx context.Context) error {
			b.logger.Debug("Client initialized")

			au := b.client.Auth()
			status, err := au.Status(ctx)
			if err != nil {
				return errors.Wrap(err, "auth status")
			}

			if !status.Authorized {
				if _, err := au.Bot(ctx, b.token); err != nil {
					return errors.Wrap(err, "login")
				}
				b.logger.Info("Bot logged in")
			} else {
				b.logger.Info("Bot login restored",
					zap.String("name", status.User.Username),
				)
			}

			if _, disableRegister := os.LookupEnv("DISABLE_COMMAND_REGISTER"); !disableRegister {
				if err := b.mux.RegisterCommands(ctx, b.raw); err != nil {
					return errors.Wrap(err, "register commands")
				}
			}

			if deployNotify := os.Getenv("TG_DEPLOY_NOTIFY_GROUP"); deployNotify != "" {
				p, err := b.sender.ResolveDomain(deployNotify, peer.OnlyChannel).AsInputPeer(ctx)
				if err != nil {
					return errors.Wrap(err, "resolve")
				}
				info, _ := debug.ReadBuildInfo()
				var commit string
				for _, c := range info.Settings {
					switch c.Key {
					case "vcs.revision":
						commit = c.Value[:7]
					}
				}
				var options []message.StyledTextOption
				options = append(options,
					styling.Plain("🚀 Started "),
					styling.Italic(fmt.Sprintf("(%s, %s, layer: %d) ",
						info.GoVersion, iapp.GetVersion(), tg.Layer),
					),
					styling.Code(commit),
				)
				if _, err := b.sender.To(p).StyledText(ctx, options...); err != nil {
					return errors.Wrap(err, "send")
				}
			}

			b.logger.Info("Bot started")
			defer func() {
				b.logger.Info("Bot stopped")
			}()

			<-ctx.Done()
			return ctx.Err()
		})
	})

	return group.Wait()
}

func runBot(ctx context.Context, m *app.Metrics, mm *iapp.Metrics, logger *zap.Logger) (rerr error) {
	a, err := InitApp(m, mm, logger)
	if err != nil {
		return errors.Wrap(err, "initialize")
	}
	defer func() {
		multierr.AppendInto(&rerr, a.Close())
	}()

	if err := setupBot(a); err != nil {
		return errors.Wrap(err, "setup")
	}

	if err := a.Run(ctx); err != nil {
		return errors.Wrap(err, "run")
	}
	return nil
}
