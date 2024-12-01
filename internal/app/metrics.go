package app

type metricWriter struct {
	Increase func(n int64) int64
	Bytes    int64
}

func (m *metricWriter) Write(p []byte) (n int, err error) {
	delta := int64(len(p))

	m.Increase(delta)
	m.Bytes += delta

	return len(p), nil
}
