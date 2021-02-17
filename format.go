package main

import (
	"strings"
	"unicode/utf8"

	"github.com/gotd/td/tg"
)

func formatMessage(s string, format func(offset, limit int) tg.MessageEntityClass) (entities []tg.MessageEntityClass) {
	length := utf8.RuneCountInString(strings.TrimSpace(s))
	return append(entities, format(0, length))
}
