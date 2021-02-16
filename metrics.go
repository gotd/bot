package main

import (
	"time"

	"go.uber.org/atomic"
)

type Metrics struct {
	Start     time.Time
	Messages  atomic.Int64
	Responses atomic.Int64
}
