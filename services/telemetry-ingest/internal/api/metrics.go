package api

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Accepted   prometheus.Counter
	Rejected   *prometheus.CounterVec
	Dropped    prometheus.Counter
	BatchSize  prometheus.Histogram
	Latency    *prometheus.HistogramVec
	StreamSubs prometheus.GaugeFunc
	LastUplift *prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer, subscribers func() float64) *Metrics {
	m := &Metrics{
		Accepted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "telemetry_samples_accepted_total", Help: "Samples validated and stored."}),
		Rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "telemetry_samples_rejected_total", Help: "Samples rejected, by reason."}, []string{"reason"}),
		Dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "telemetry_stream_dropped_total", Help: "Samples not delivered to a slow live subscriber."}),
		BatchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telemetry_batch_size", Help: "Samples per ingest request.",
			Buckets: prometheus.ExponentialBuckets(1, 4, 7)}),
		Latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "HTTP latency by route and status.",
			Buckets: prometheus.DefBuckets}, []string{"route", "code"}),
		StreamSubs: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "telemetry_stream_subscribers", Help: "Open SSE connections."}, subscribers),
		LastUplift: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "robot_surface_uplift_mm", Help: "Latest reported surface uplift per robot."}, []string{"site", "robot"}),
	}
	reg.MustRegister(m.Accepted, m.Rejected, m.Dropped, m.BatchSize, m.Latency, m.StreamSubs, m.LastUplift)
	return m
}
