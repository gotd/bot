package tgmanager

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_extractCode(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		Message string
		Result  string
	}{
		{
			Name:    "Empty",
			Message: "",
			Result:  "",
		},
		{
			Name: "English",
			Message: `Login code: 70021. Do not give this code to anyone, even if they say they are from Telegram!

❗️This code can be used to log in to your Telegram account. We never ask it for anything else.

If you didn't request this code by trying to log in on another device, simply ignore this message.`,
			Result: "70021",
		},
		{
			Name: "NonCode",
			Message: `New login. Dear Foo, we detected a login into your account from a new device on 08/12/2024 at 11:43:00 UTC.

Device: tdesktop, v0.115.0, go1.23.3, Desktop, linux
Location: Russia

If this wasn't you, you can terminate that session in Settings > Devices (or Privacy & Security > Active Sessions).

If you think that somebody logged in to your account against your will, you can enable Two-Step Verification in Privacy and Security settings.`,
			Result: "",
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			got := extractCode(tt.Message)
			require.Equal(t, tt.Result, got)
		})
	}
}
