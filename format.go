package main

import (
	"strings"
	"unicode/utf8"

	"github.com/gotd/td/tg"
)

func formatMessage(s string, format func(offset, limit int) tg.MessageEntityClass) (entities []tg.MessageEntityClass) {
	offset := 0
	s = strings.TrimSpace(s)
	for {
		idx := strings.IndexRune(s[offset:], '\n')
		if idx < 0 {
			lineLength := utf8.RuneCountInString(s[offset:])
			entities = append(entities, format(offset, lineLength))
			break
		}
		idx++ // Add \n.

		entities = append(entities, format(offset, idx))
		offset += idx
	}

	return
}
