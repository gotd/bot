package net

import "context"

// Net is interface for GPT network
type Net interface {
	Query(ctx context.Context, query string) (string, error)
}
