package metrics

import (
	"net/http"

	"go.uber.org/zap"
)

// MiddlewareOptions is middleware options.
type MiddlewareOptions struct {
	Token      string
	HTTPClient *http.Client
	Logger     *zap.Logger
}

func (m *MiddlewareOptions) setDefaults() {
	if m.HTTPClient == nil {
		m.HTTPClient = http.DefaultClient
	}
	if m.Logger == nil {
		m.Logger = zap.NewNop()
	}
}
