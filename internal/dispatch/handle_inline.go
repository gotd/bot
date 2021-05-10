package dispatch

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/gotd/td/tg"
)

func (b *Bot) OnBotInlineQuery(ctx context.Context, e tg.Entities, u *tg.UpdateBotInlineQuery) error {
	user, ok := e.Users[u.UserID]
	if !ok {
		return xerrors.Errorf("unknown user ID %d", u.UserID)
	}

	var geo *tg.GeoPoint
	if u.Geo != nil {
		geo, _ = u.Geo.AsNotEmpty()
	}
	return b.onInline.OnInline(ctx, InlineQuery{
		QueryID:   u.QueryID,
		Query:     u.Query,
		Offset:    u.Offset,
		Enquirer:  user.AsInput(),
		geo:       geo,
		user:      user,
		baseEvent: b.baseEvent(),
	})
}
