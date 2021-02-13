// Binary bot provides implementation of gotd bot.
package main

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/mtproto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func tokHash(token string) string {
	h := md5.Sum([]byte(token + "gotd-token-salt")) // #nosec
	return fmt.Sprintf("%x", h[:5])
}

func sessionFileName(token string) string {
	return fmt.Sprintf("bot.%s.session.json", tokHash(token))
}

// State represents current state.
type State struct {
	mux sync.Mutex
	pts int

	db  *pebble.DB
	log *zap.Logger
}

type StateUpdate struct {
	Remote int
	Local  int
}

// Commit offset, like in Kafka.
func (s *State) Commit(pts int) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.pts >= pts {
		return nil
	}

	if err := s.set(pts); err != nil {
		return xerrors.Errorf("set: %w", err)
	}
	s.pts = pts

	s.log.Debug("Commit", zap.Int("pts", pts))

	return nil
}

// Sync to remote pts. If not observed, applyUpdate is called.
func (s *State) Sync(remoteTimeStamp int, applyUpdate func(upd StateUpdate) error) error {
	s.mux.Lock()
	syncNeeded := s.pts < remoteTimeStamp
	s.mux.Unlock()

	if s.pts == 0 {
		// Got initial state.
		if err := s.Commit(remoteTimeStamp); err != nil {
			return xerrors.Errorf("commit init state: %w", err)
		}
		return nil
	}

	if !syncNeeded {
		return nil
	}

	if err := applyUpdate(StateUpdate{Remote: remoteTimeStamp, Local: s.pts}); err != nil {
		return xerrors.Errorf("apply: %w", err)
	}
	if err := s.Commit(remoteTimeStamp); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}

	return nil
}

func (s *State) set(pts int) error {

	var b bin.Buffer
	b.PutInt(pts)
	if err := s.db.Set([]byte("pts"), b.Buf, nil); err != nil {
		return xerrors.Errorf("put: %w", err)
	}

	s.pts = pts
	s.log.Info("Updated local state", zap.Int("pts", pts))

	return nil
}

func (s *State) Load() error {
	v, closer, err := s.db.Get([]byte("pts"))
	if errors.Is(err, pebble.ErrNotFound) {
		// No state.
		s.pts = 0
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get: %w", err)
	}
	defer func() { _ = closer.Close() }()

	b := bin.Buffer{Buf: v}
	n, err := b.Int()
	if err != nil {
		return xerrors.Errorf("failed to get long")
	}
	s.pts = n

	return nil
}

func withSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}

type loggedDispatcher struct {
	telegram.UpdateHandler
	log *zap.Logger
}

func (d loggedDispatcher) Handle(ctx context.Context, updates *tg.Updates) error {
	for _, u := range updates.Updates {
		d.log.Debug("Update",
			zap.String("t", fmt.Sprintf("%T", u)),
		)
	}
	return d.UpdateHandler.Handle(ctx, updates)
}

func (d loggedDispatcher) HandleShort(ctx context.Context, u *tg.UpdateShort) error {
	d.log.Debug("UpdateShort",
		zap.String("t", fmt.Sprintf("%T", u.Update)),
	)
	return d.UpdateHandler.HandleShort(ctx, u)
}

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

	dispatcher.OnNewChannelMessage(func(ctx tg.UpdateContext, u *tg.UpdateNewChannelMessage) error {
		switch m := u.Message.(type) {
		case *tg.Message:
			if m.Out {
				return nil
			}
			logger.With(
				zap.String("text", m.Message),
			).Info("Got message from channel")
		}
		return nil
	})
	dispatcher.OnNewMessage(func(ctx tg.UpdateContext, u *tg.UpdateNewMessage) error {
		switch m := u.Message.(type) {
		case *tg.Message:
			if m.Out {
				return nil
			}

			switch peer := m.PeerID.(type) {
			case *tg.PeerUser:
				user := ctx.Users[peer.UserID]
				logger.With(
					zap.String("text", m.Message),
					zap.Int("user_id", user.ID),
					zap.String("user_first_name", user.FirstName),
					zap.String("username", user.Username),
				).Info("Got message")

				reply := &tg.MessagesSendMessageRequest{
					Message: fmt.Sprintf("No u %s, @%s", m.Message, user.Username),
					Peer: &tg.InputPeerUser{
						UserID:     user.ID,
						AccessHash: user.AccessHash,
					},
				}
				reply.SetReplyToMsgID(m.ID)

				if err := client.SendMessage(ctx, reply); err != nil {
					if mtproto.IsErr(err, tg.ErrUserIsBlocked) {
						logger.Debug("Bot is blocked by user")
						return nil
					}

					return xerrors.Errorf("send message: %w", err)
				}
			}
		}

		if err := state.Commit(u.Pts); err != nil {
			return xerrors.Errorf("commit: %w", err)
		}
		return nil
	})

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
	ctx, cancel := withSignal(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(2)
	}
}
