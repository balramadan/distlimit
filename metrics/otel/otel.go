// Package otel provides an OpenTelemetry metrics observer implementation for distlimit.
package otel

import (
	"context"

	"github.com/balramadan/distlimit/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metric "go.opentelemetry.io/otel/metric"
)

// Observer implements the metrics.Observer interface by recording rate limit evaluation metrics into OpenTelemetry meters.
type Observer struct {
	requestsCounter    metric.Int64Counter
	durationHistogram  metric.Float64Histogram
	fallbackCounter    metric.Int64Counter
}

// Option configures functional parameters for the OpenTelemetry Observer.
type Option func(*config)

type config struct {
	meterProvider metric.MeterProvider
	meterName     string
}

// WithMeterProvider sets a custom OpenTelemetry MeterProvider. Defaults to otel.GetMeterProvider().
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *config) {
		if mp != nil {
			c.meterProvider = mp
		}
	}
}

// WithMeterName sets a custom OpenTelemetry Meter name. Default is "github.com/balramadan/distlimit".
func WithMeterName(name string) Option {
	return func(c *config) {
		c.meterName = name
	}
}

// NewObserver creates and initializes a new OpenTelemetry metrics Observer.
func NewObserver(opts ...Option) (*Observer, error) {
	cfg := &config{
		meterProvider: otel.GetMeterProvider(),
		meterName:     "github.com/balramadan/distlimit",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	meter := cfg.meterProvider.Meter(cfg.meterName)

	requestsCounter, err := meter.Int64Counter(
		"distlimit.requests.total",
		metric.WithDescription("Total number of rate limit evaluations."),
	)
	if err != nil {
		return nil, err
	}

	durationHistogram, err := meter.Float64Histogram(
		"distlimit.evaluation.duration.seconds",
		metric.WithDescription("Histogram of rate limit evaluation latencies in seconds."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	fallbackCounter, err := meter.Int64Counter(
		"distlimit.hybrid.fallback.total",
		metric.WithDescription("Total number of hybrid driver primary storage failures triggering fallback execution."),
	)
	if err != nil {
		return nil, err
	}

	return &Observer{
		requestsCounter:   requestsCounter,
		durationHistogram: durationHistogram,
		fallbackCounter:   fallbackCounter,
	}, nil
}

// Observe records a rate limit evaluation event to OpenTelemetry counters and histograms.
func (o *Observer) Observe(ctx context.Context, event metrics.Event) {
	status := "allowed"
	if !event.Allowed {
		status = "blocked"
	}

	attrs := metric.WithAttributes(
		attribute.String("driver", event.Driver),
		attribute.String("algorithm", event.Algorithm),
		attribute.String("status", status),
	)

	o.requestsCounter.Add(ctx, 1, attrs)

	histAttrs := metric.WithAttributes(
		attribute.String("driver", event.Driver),
		attribute.String("algorithm", event.Algorithm),
	)

	o.durationHistogram.Record(ctx, event.Duration.Seconds(), histAttrs)
}

// RecordHybridFallback increments the hybrid driver fallback metric counter.
func (o *Observer) RecordHybridFallback(ctx context.Context, reason string) {
	attrs := metric.WithAttributes(
		attribute.String("reason", reason),
	)
	o.fallbackCounter.Add(ctx, 1, attrs)
}
