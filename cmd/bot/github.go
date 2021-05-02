package main

import (
	"encoding/base64"
	"net/http"
	"os"
	"strconv"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v33/github"
	"golang.org/x/xerrors"
)

func setupGithub(appID string, httpTransport http.RoundTripper) (*github.Client, error) {
	ghAppID, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return nil, xerrors.Errorf("GITHUB_APP_ID is invalid: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("GITHUB_PRIVATE_KEY"))
	if err != nil {
		return nil, xerrors.Errorf("GITHUB_PRIVATE_KEY is invalid: %w", err)
	}
	ghTransport, err := ghinstallation.NewAppsTransport(httpTransport, ghAppID, key)
	if err != nil {
		return nil, xerrors.Errorf("create github transport: %w", err)
	}
	return github.NewClient(&http.Client{
		Transport: ghTransport,
	}), nil
}
