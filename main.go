// Binary bot provides implementation of gotd bot.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/pebble"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func run(ctx context.Context) (err error) {
	logger, _ := zap.NewDevelopment(
		zap.IncreaseLevel(zapcore.DebugLevel),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	defer func() { _ = logger.Sync() }()

	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return xerrors.Errorf("APP_ID not set or invalid: %w", err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return xerrors.New("no APP_HASH provided")
	}

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
	bot := NewBot(state, client).
		WithLogger(logger.Named("bot")).
		WithStart(time.Now())
	dispatcher.OnNewMessage(bot.OnNewMessage)
	dispatcher.OnNewChannelMessage(bot.OnNewChannelMessage)

	return client.Run(ctx, func(ctx context.Context) error {
		logger.Debug("Client initialized")

		self, err := client.Self(ctx)
		if err != nil || !self.Bot {
			if err := client.AuthBot(ctx, token); err != nil {
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
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(2)
	}
}
