package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
	distlimithybrid "github.com/balramadan/distlimit/driver/hybrid"
	distlimitmemory "github.com/balramadan/distlimit/driver/memory"
	distlimitredis "github.com/balramadan/distlimit/driver/redis"
	distlimitgin "github.com/balramadan/distlimit/middleware/gin"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Primary Driver: Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	redisDriver := distlimitredis.New(rdb)

	// 2. Fallback Driver: Memory (64-Sharded)
	memDriver := distlimitmemory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	// 3. Dual-Tier Hybrid Driver dengan Half-Open Circuit Breaker
	hybridDriver := distlimithybrid.New(
		redisDriver,
		memDriver,
		distlimithybrid.WithCoolOffDuration(5*time.Second),
		distlimithybrid.WithOnError(func(err error) {
			log.Printf("[DISTLIMIT WARN] Redis Primary down/error: %v. Switching to Memory Fallback.", err)
		}),
	)

	// 4. Inisialisasi Limiter dengan Hybrid Driver & Token Bucket Algorithm
	limiter, err := distlimit.New(
		hybridDriver,
		distlimit.WithLimit(10),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	r := gin.Default()

	// Pasang Middleware Gin
	r.Use(distlimitgin.New(limiter))

	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "pong from Gin with Hybrid Driver!",
		})
	})

	log.Println("⚡ Gin + Hybrid Driver server running on http://localhost:8080")
	log.Fatal(r.Run(":8080"))
}
