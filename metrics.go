package main

import (
	"time"

	"go.uber.org/atomic"
)

type Metrics struct {
	Start      time.Time
	Messages   atomic.Int64
	Responses  atomic.Int64
	MediaBytes atomic.Int64
}

type metricWriter struct {
	Bytes int64
}

func (m *metricWriter) Write(p []byte) (n int, err error) {
	m.Bytes += int64(len(p))

	return len(p), nil
}
