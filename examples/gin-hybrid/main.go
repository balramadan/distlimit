package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitgin "github.com/balramadan/distlimit/middleware/gin"
	"github.com/gin-gonic/gin"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(context.Background())

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(10),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	r := gin.Default()
	r.Use(distlimitgin.New(limiter))

	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "pong from Gin!",
		})
	})

	log.Println("⚡ Gin server running on http://localhost:8080")
	log.Fatal(r.Run(":8080"))
}
