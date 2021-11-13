package docs

import (
	"context"
	"fmt"
	escapehtml "html"
	"strings"

	"github.com/go-faster/errors"
	"go.uber.org/multierr"

	"github.com/gotd/getdoc"
	"github.com/gotd/td/telegram/message/entity"
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

func writeType(w *strings.Builder, typ tl.Type, namespace []string, text string) {
	if typ.Bare || typ.GenericRef || typ.GenericArg != nil {
		w.WriteString(text)
		return
	}

	w.WriteString("</pre>")
	w.WriteString(fmt.Sprintf(
		`<a href="https://core.telegram.org/type/%s">`,
		escapehtml.EscapeString(namespacedName(typ.Name, namespace)),
	))
	w.WriteString(text)
	w.WriteString("</a>")
	w.WriteString("<pre>")
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
		escaped := escapehtml.EscapeString(param.String())
		if param.Flags {
			b.WriteString(escaped)
			continue
		}
		writeType(&b, param.Type, nil, escaped)
	}
	if d.Base {
		b.WriteString(" ?")
	}
	b.WriteString(" = ")
	writeType(&b, d.Type, d.Namespace, escapehtml.EscapeString(d.Type.String()))
	b.WriteString("</pre>")

	return html.String(nil, b.String())
}

// OnInline implements dispatch.InlineHandler.
func (h Handler) OnInline(ctx context.Context, e dispatch.InlineQuery) error {
	reply := e.Reply()

	results, err := h.search.Match(e.Query)
	if err != nil {
		_, setErr := reply.Set(ctx)
		return multierr.Append(errors.Wrapf(setErr, "search"), err)
	}

	var options []inline.ResultOption
	for _, result := range results {
		def := result.Definition
		title := fmt.Sprintf("%s %s#%x", result.Category.String(), def.Name, def.ID)
		goDoc := fmt.Sprintf("https://ref.gotd.dev/use/github.com/gotd/td/tg..%s.html", result.GoName)

		var (
			desc      []string
			docURL    string
			fields    map[string]getdoc.ParamDescription
			botCanUse bool
		)
		switch result.Category {
		case tl.CategoryType:
			desc = result.Constructor.Description
			docURL = fmt.Sprintf("https://core.telegram.org/constructor/%s", result.NamespacedName)
			fields = result.Constructor.Fields
		case tl.CategoryFunction:
			desc = result.Method.Description
			docURL = fmt.Sprintf("https://core.telegram.org/method/%s", result.NamespacedName)
			fields = result.Method.Parameters
		}
		description := strings.Join(desc, " ")

		msg := inline.MessageStyledText(
			formatDefinition(def),
			styling.Custom(func(eb *entity.Builder) error {
				eb.Plain("\n\n")
				eb.Italic(description)
				eb.Plain("\n\n")

				if botCanUse {
					eb.Plain("\n\n")
					eb.Plain("Bot can use this method")
					eb.Plain("\n\n")
				}

				for _, field := range fields {
					eb.Plain("-")
					eb.Bold(field.Name)
					eb.Plain(" ")
					eb.Italic(field.Description)
					eb.Plain("\n")
				}
				eb.Plain("\n")

				return nil
			}),
		).Row(
			markup.URL("Telegram docs", docURL),
			markup.URL("gotd docs", goDoc),
		).NoWebpage()

		options = append(options, inline.Article(title, msg).Description(description))
	}
	_, err = e.Reply().Set(ctx, options...)
	return err
}
