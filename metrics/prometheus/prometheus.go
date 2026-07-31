// Package prometheus provides a Prometheus metrics observer implementation for distlimit.
package prometheus

import (
	"context"

	"github.com/balramadan/distlimit/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Observer implements the metrics.Observer interface by recording rate limit metrics into Prometheus collectors.
type Observer struct {
	requestsTotal       *prometheus.CounterVec
	evaluationDuration  *prometheus.HistogramVec
	hybridFallbackTotal *prometheus.CounterVec
}

// Option configures functional parameters for the Prometheus Observer.
type Option func(*config)

type config struct {
	namespace  string
	subsystem  string
	registerer prometheus.Registerer
}

// WithNamespace sets a custom Prometheus namespace prefix for all metrics.
func WithNamespace(ns string) Option {
	return func(c *config) {
		c.namespace = ns
	}
}

// WithSubsystem sets a custom Prometheus subsystem prefix for all metrics. Default is "rate_limit".
func WithSubsystem(sub string) Option {
	return func(c *config) {
		c.subsystem = sub
	}
}

// WithRegisterer sets a custom Prometheus Registerer. Defaults to prometheus.DefaultRegisterer.
func WithRegisterer(reg prometheus.Registerer) Option {
	return func(c *config) {
		if reg != nil {
			c.registerer = reg
		}
	}
}

// NewObserver creates and registers a new Prometheus metrics Observer with distlimit collectors.
func NewObserver(opts ...Option) *Observer {
	cfg := &config{
		namespace:  "distlimit",
		subsystem:  "rate_limit",
		registerer: prometheus.DefaultRegisterer,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	obs := &Observer{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.namespace,
				Subsystem: cfg.subsystem,
				Name:      "requests_total",
				Help:      "Total number of rate limit evaluations.",
			},
			[]string{"driver", "algorithm", "status"},
		),
		evaluationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.namespace,
				Subsystem: cfg.subsystem,
				Name:      "evaluation_duration_seconds",
				Help:      "Histogram of rate limit evaluation latencies in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"driver", "algorithm"},
		),
		hybridFallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.namespace,
				Subsystem: cfg.subsystem,
				Name:      "hybrid_fallback_total",
				Help:      "Total number of hybrid driver primary storage failures triggering fallback execution.",
			},
			[]string{"reason"},
		),
	}

	_ = cfg.registerer.Register(obs.requestsTotal)
	_ = cfg.registerer.Register(obs.evaluationDuration)
	_ = cfg.registerer.Register(obs.hybridFallbackTotal)

	return obs
}

// Observe records a rate limit evaluation event to Prometheus counters and histograms.
func (o *Observer) Observe(ctx context.Context, event metrics.Event) {
	status := "allowed"
	if !event.Allowed {
		status = "blocked"
	}

	o.requestsTotal.WithLabelValues(event.Driver, event.Algorithm, status).Inc()
	o.evaluationDuration.WithLabelValues(event.Driver, event.Algorithm).Observe(event.Duration.Seconds())
}

// RecordHybridFallback increments the hybrid driver fallback metric counter.
func (o *Observer) RecordHybridFallback(reason string) {
	o.hybridFallbackTotal.WithLabelValues(reason).Inc()
}
