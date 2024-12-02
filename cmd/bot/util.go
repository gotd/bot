package main

import (
	"crypto/md5"
	"fmt"
)

func tokHash(token string) string {
	h := md5.Sum([]byte(token + "gotd-token-salt")) // #nosec
	return fmt.Sprintf("%x", h[:5])
}
