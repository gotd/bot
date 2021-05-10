package docs

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/inline"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/tl"

	"github.com/gotd/bot/internal/dispatch"
)

// Handler implements docs inline query handler.
type Handler struct {
	search *Search
}

// New creates new Handler.
func New(search *Search) Handler {
	return Handler{search: search}
}

func formatDefinition(d tl.Definition) styling.StyledTextOption {
	var b strings.Builder

	b.WriteString("<pre>")
	for _, ns := range d.Namespace {
		b.WriteString(ns)
		b.WriteRune('.')
	}
	b.WriteString(fmt.Sprintf("%s#%x", d.Name, d.ID))
	for _, param := range d.GenericParams {
		b.WriteString(" {")
		b.WriteString(param)
		b.WriteString(":Type}")
	}
	for _, param := range d.Params {
		b.WriteRune(' ')
		paramString := param.String()
		if param.Type.Bare || param.Type.GenericRef || param.Type.GenericArg != nil {
			b.WriteString(paramString)
			continue
		}

		b.WriteString("</pre>")
		b.WriteString(fmt.Sprintf(`<a href="https://core.telegram.org/type/%s">`, param.Type.Name))
		b.WriteString(param.String())
		b.WriteString("</a>")
		b.WriteString("<pre>")
	}
	if d.Base {
		b.WriteString(" ?")
	}
	b.WriteString(" = ")
	b.WriteString(d.Type.String())
	b.WriteString("</pre>")

	return html.String(nil, b.String())
}

// OnInline implements dispatch.InlineHandler.
func (h Handler) OnInline(ctx context.Context, e dispatch.InlineQuery) error {
	reply := e.Reply()

	results, err := h.search.Match(e.Query)
	if err != nil {
		_, rErr := reply.Set(ctx)
		return multierr.Append(xerrors.Errorf("search: %w", err), rErr)
	}

	var options []inline.ResultOption
	for _, result := range results {
		def := result.Definition
		title := fmt.Sprintf("%s %s#%x", result.Category.String(), def.Name, def.ID)
		goDoc := fmt.Sprintf("https://ref.gotd.dev/use/github.com/gotd/td/tg..%s.html", result.GoName)

		var (
			desc   []string
			docURL string
		)
		switch result.Category {
		case tl.CategoryType:
			desc = result.Constructor.Description
			docURL = fmt.Sprintf("https://core.telegram.org/constructor/%s", result.NamespacedName)
		case tl.CategoryFunction:
			desc = result.Method.Description
			docURL = fmt.Sprintf("https://core.telegram.org/method/%s", result.NamespacedName)
		}
		description := strings.Join(desc, " ")

		msg := inline.MessageStyledText(
			formatDefinition(def),
			styling.Plain("\n\n"),
			styling.Italic(description),
		).Row(
			markup.URL("Telegram docs", docURL),
			markup.URL("gotd docs", goDoc),
		)
		options = append(options, inline.Article(title, msg.NoWebpage()).Description(description))
	}
	_, err = e.Reply().Set(ctx, options...)
	return err
}
