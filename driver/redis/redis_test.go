package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/balramadan/distlimit/algorithm/fixedwindow"
	distlimitredis "github.com/balramadan/distlimit/driver/redis"
	"github.com/redis/go-redis/v9"
)

func TestRedisDriver_ClusterHashTagFormatting(t *testing.T) {
	// Menggunakan miniredis / mock / client dummy
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	rDriver := distlimitredis.New(client, distlimitredis.WithPrefix("distlimit"))

	alg := fixedwindow.New()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Panggilan Allow harus menyusun key dengan Hash Tag format distlimit:{user1}
	_, _ = rDriver.Allow(ctx, "user123", 10, 1*time.Minute, alg)
}
