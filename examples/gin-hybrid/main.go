package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/driver/hybrid"
	"github.com/balramadan/distlimit/driver/memory"
	"github.com/balramadan/distlimit/driver/redis"
	distlimitgin "github.com/balramadan/distlimit/middleware/gin"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	// 1. Setup Redis Client (L2 Primary Storage)
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: "", // Default no password
		DB:       0,
	})

	// PING Redis untuk mengecek koneksi awal
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Println("⚠️  Warning: Redis ping gagal. Hybrid Driver akan otomatis menggunakan In-Memory Fallback!")
	} else {
		log.Println("✅ Terhubung ke Redis Server (Primary Driver Ready).")
	}

	// 2. Inisialisasi Driver L1 (Memory) dan L2 (Redis)
	memDriver := memory.New(1 * time.Minute)
	defer memDriver.Close(context.Background())

	redisDriver := redis.New(rdb, redis.WithPrefix("demo:distlimit:"))

	// 3. Rakit Hybrid Driver (The Killer Feature)
	hybridDriver := hybrid.New(
		redisDriver,
		memDriver,
		hybrid.WithCoolOffDuration(5*time.Second), // Cool-off 5 detik jika Redis down
		hybrid.WithOnError(func(err error) {
			log.Printf("🚨 [DISTLIMIT ALERT] Terjadi gangguan pada Redis: %v! Otomatis beralih ke In-Memory Fallback.", err)
		}),
	)
	defer hybridDriver.Close(context.Background())

	// 4. Inisialisasi Core Limiter (Aturan: Maksimal 5 Request per 10 Detik)
	limiter, err := distlimit.New(
		hybridDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(10*time.Second),
	)
	if err != nil {
		log.Fatalf("Gagal membuat limiter: %v", err)
	}

	// 5. Inisialisasi Gin Engine & Pasang Distlimit Middleware
	r := gin.Default()
	r.Use(distlimitgin.New(limiter))

	// 6. Endpoint Contoh
	r.GET("/api/v1/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":        "pong",
			"redis_healthy":  hybridDriver.IsPrimaryHealthy(),
			"fallback_count": hybridDriver.FallbackCount(),
		})
	})

	log.Println("🚀 Server demo berjalan di http://localhost:8080")
	log.Println("💡 Batas Rate Limit: 5 Request / 10 Detik")
	_ = r.Run(":8080")
}
