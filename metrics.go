package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

type metric struct {
	atomic.Int64
	prometheus.CounterFunc
}

func newMetric(opts prometheus.CounterOpts) *metric {
	m := &metric{}
	m.CounterFunc = prometheus.NewCounterFunc(opts, func() float64 {
		return float64(m.Load())
	})
	return m
}

type Metrics struct {
	Start      time.Time
	Messages   *metric
	Responses  *metric
	MediaBytes *metric
}

func (m Metrics) Describe(desc chan<- *prometheus.Desc) {
	m.Messages.Describe(desc)
	m.Responses.Describe(desc)
	m.MediaBytes.Describe(desc)
}

func (m Metrics) Collect(ch chan<- prometheus.Metric) {
	m.Messages.Collect(ch)
	m.Responses.Collect(ch)
	m.MediaBytes.Collect(ch)
}

func NewMetrics() Metrics {
	return Metrics{
		Messages: newMetric(prometheus.CounterOpts{
			Name: "bot_messages",
			Help: "Total count of received messages",
		}),
		Responses: newMetric(prometheus.CounterOpts{
			Name: "bot_responses",
			Help: "Total count of answered messages",
		}),
		MediaBytes: newMetric(prometheus.CounterOpts{
			Name: "bot_media_bytes",
			Help: "Total count of received media bytes",
		}),
	}
}

type metricWriter struct {
	Bytes int64
}

func (m *metricWriter) Write(p []byte) (n int, err error) {
	m.Bytes += int64(len(p))

	return len(p), nil
}
