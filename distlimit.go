package distlimit

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidLimit = errors.New("Distlimit: The limit must be greater than 0")

var ErrInvalidWindow = errors.New("Distlimit: The window must be greater than 0")

var ErrNilDriver = errors.New("Distlimit: The driver must not be nil")

type Result struct {
	Allowed   bool
	Limit     int64
	Remaining int64
	ResetIn   time.Duration
}

type Driver interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (Result, error)

	Close(ctx context.Context) error
}

type KeyFunc func(ctx context.Context) string

type Limiter struct {
	driver  Driver
	limit   int64
	window  time.Duration
	keyFunc KeyFunc
}

type Option func(*Limiter)

func WithLimit(limit int64) Option {
	return func(l *Limiter) {
		l.limit = limit
	}
}

func WithWindow(window time.Duration) Option {
	return func(l *Limiter) {
		l.window = window
	}
}

func WithKeyFunc(fn KeyFunc) Option {
	return func(l *Limiter) {
		l.keyFunc = fn
	}
}

func New(driver Driver, opts ...Option) (*Limiter, error) {
	if driver == nil {
		return nil, ErrNilDriver
	}

	limiter := &Limiter{
		driver: driver,
		limit:  100,
		window: 1 * time.Minute,
		keyFunc: func(ctx context.Context) string {
			return "global"
		},
	}

	for _, opt := range opts {
		opt(limiter)
	}

	if limiter.limit <= 0 {
		return nil, ErrInvalidLimit
	}
	if limiter.window <= 0 {
		return nil, ErrInvalidWindow
	}

	return limiter, nil
}

func (l *Limiter) Allow(ctx context.Context) (Result, error) {
	key := l.keyFunc(ctx)
	return l.driver.Allow(ctx, key, l.limit, l.window)
}

func (l *Limiter) AllowKey(ctx context.Context, key string) (Result, error) {
	return l.driver.Allow(ctx, key, l.limit, l.window)
}

func (l *Limiter) Close(ctx context.Context) error {
	return l.driver.Close(ctx)
}
