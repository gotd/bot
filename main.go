// Binary bot provides implementation of gotd bot.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/cockroachdb/pebble"
	"github.com/google/go-github/v33/github"
	"github.com/povilasv/prommod"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/gotd/bot/net"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func bot(ctx context.Context, metrics Metrics, logger *zap.Logger) error {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return xerrors.New("no BOT_TOKEN provided")
	}

	// Setting up session storage.
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sessionDir := filepath.Join(home, ".td")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return err
	}

	db, err := pebble.Open(
		filepath.Join(sessionDir, fmt.Sprintf("bot.%s.state", tokHash(token))),
		&pebble.Options{},
	)
	if err != nil {
		return xerrors.Errorf("database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	state := &State{db: db, log: logger.Named("state")}
	if err := state.Load(); err != nil {
		return xerrors.Errorf("state load: %w", err)
	}

	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return xerrors.Errorf("APP_ID not set or invalid: %w", err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return xerrors.New("no APP_HASH provided")
	}

	g := net.NewGPT2()
	if v := os.Getenv("GPT_MIN_SIZE"); v != "" {
		min, err := strconv.Atoi(v)
		if err != nil {
			return xerrors.Errorf("GPT_MIN_SIZE %q is invalid: %w", v, err)
		}
		g = g.WithMinLength(min)
	}
	if v := os.Getenv("GPT_MAX_SIZE"); v != "" {
		max, err := strconv.Atoi(v)
		if err != nil {
			return xerrors.Errorf("GPT_MAX_SIZE %q is invalid: %w", v, err)
		}
		g = g.WithMaxLength(max)
	}

	dispatcher := tg.NewUpdateDispatcher()
	client := telegram.NewClient(appID, appHash, telegram.Options{
		Logger: logger,
		SessionStorage: &session.FileStorage{
			Path: filepath.Join(sessionDir, sessionFileName(token)),
		},
		UpdateHandler: loggedDispatcher{
			log:           logger.Named("updates"),
			UpdateHandler: dispatcher,
		},
	})
	bot := NewBot(state, client, metrics).
		WithLogger(logger.Named("bot")).
		WithStart(time.Now()).
		WithGPT2(g)

	group, ctx := errgroup.WithContext(ctx)

	if v := os.Getenv("GITHUB_APP_ID"); v != "" {
		ghAppID, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return xerrors.Errorf("GITHUB_APP_ID is invalid: %w", err)
		}
		key, err := base64.StdEncoding.DecodeString(os.Getenv("GITHUB_PRIVATE_KEY"))
		if err != nil {
			return xerrors.Errorf("GITHUB_PRIVATE_KEY is invalid: %w", err)
		}
		ghTransport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, ghAppID, key)
		if err != nil {
			return xerrors.Errorf("create github transport: %w", err)
		}
		ghClient := github.NewClient(&http.Client{
			Transport: ghTransport,
		})
		bot.WithGithub(ghClient)
		bot.WithGithubSecret(os.Getenv("GITHUB_SECRET"))

		httpAddr := os.Getenv("HTTP_ADDR")
		if httpAddr == "" {
			httpAddr = "localhost:8080"
		}
		mux := http.NewServeMux()
		bot.RegisterRoutes(mux)
		server := http.Server{
			Addr:    httpAddr,
			Handler: mux,
		}
		group.Go(func() error {
			return server.ListenAndServe()
		})
		group.Go(func() error {
			<-ctx.Done()
			shutCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			if err := server.Shutdown(shutCtx); err != nil {
				_ = server.Close()
			}

			return nil
		})
	}
	dispatcher.OnNewMessage(bot.OnNewMessage)
	dispatcher.OnNewChannelMessage(bot.OnNewChannelMessage)

	group.Go(func() error {
		return client.Run(ctx, func(ctx context.Context) error {
			logger.Debug("Client initialized")

			self, err := client.Self(ctx)
			if err != nil || !self.Bot {
				if _, err := client.AuthBot(ctx, token); err != nil {
					return xerrors.Errorf("failed to perform bot login: %w", err)
				}
				logger.Info("New bot login")
			} else {
				logger.Info("Bot login  restored",
					zap.String("name", self.Username),
				)
			}

			// Using tg.Client for directly calling RPC.
			raw := tg.NewClient(client)

			// Syncing with remote state.
			remoteState, err := raw.UpdatesGetState(ctx)
			if err != nil {
				return xerrors.Errorf("failed to get state: %w", err)
			}
			logger.Info("Got state",
				zap.Int("qts", remoteState.Qts),
				zap.Int("pts", remoteState.Pts),
				zap.Int("seq", remoteState.Seq),
				zap.Int("unread_count", remoteState.UnreadCount),
			)

			if err := state.Sync(remoteState.Pts, func(upd StateUpdate) error {
				logger.Info("Applying updates",
					zap.Int("remote_pts", upd.Remote),
					zap.Int("local_pts", upd.Local),
				)

				diff, err := raw.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{
					Date: int(time.Now().Unix()),
					Pts:  upd.Local,
					Qts:  0, // No secret chats
				})
				if err != nil {
					return xerrors.Errorf("get difference: %w", err)
				}

				switch d := diff.(type) {
				case *tg.UpdatesDifference:
					// Adapting update to Handle() input.
					var updates []tg.UpdateClass
					for _, u := range d.OtherUpdates {
						updates = append(updates, u)
					}
					for _, m := range d.NewMessages {
						updates = append(updates, &tg.UpdateNewMessage{
							Message: m,

							// We can't provide pts here.
							Pts:      0,
							PtsCount: 0,
						})
					}
					if err := dispatcher.Handle(ctx, &tg.Updates{
						Updates: updates,
						Users:   d.Users,
						Chats:   d.Chats,
					}); err != nil {
						return xerrors.Errorf("handle: %w", err)
					}
				case *tg.UpdatesDifferenceSlice:
					logger.Warn("Ignoring difference slice")
				default:
					logger.Warn("Ignoring updates")
				}

				logger.Info("Update handled")

				return nil
			}); err != nil {
				return xerrors.Errorf("sync: %w", err)
			}

			<-ctx.Done()
			return ctx.Err()
		})
	})

	return group.Wait()
}

func run(ctx context.Context) (err error) {
	logger, _ := zap.NewDevelopment(
		zap.IncreaseLevel(zapcore.DebugLevel),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	defer func() { _ = logger.Sync() }()

	registry := prometheus.NewPedanticRegistry()
	mts := NewMetrics()
	registry.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		prommod.NewCollector("gotdbot"),
		mts,
	)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return bot(ctx, mts, logger.Named("bot"))
	})

	g.Go(func() error {
		return metrics(ctx, registry, logger.Named("metrics"))
	})

	return g.Wait()
}

func attachProfiler(router *http.ServeMux) {
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Manually add support for paths linked to by index page at /debug/pprof/
	router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	router.Handle("/debug/pprof/block", pprof.Handler("block"))
}

func metrics(ctx context.Context, registry *prometheus.Registry, logger *zap.Logger) error {
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "localhost:8081"
	}
	mux := http.NewServeMux()
	attachProfiler(mux)
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := &http.Server{Addr: metricsAddr, Handler: mux}

	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		logger.Info("ListenAndServe", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	grp.Go(func() error {
		<-ctx.Done()
		logger.Debug("Shutting down")
		return server.Close()
	})

	return grp.Wait()
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(2)
	}
}
