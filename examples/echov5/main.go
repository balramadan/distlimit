package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/fixedwindow"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitechov5 "github.com/balramadan/distlimit/middleware/echov5"
	"github.com/labstack/echo/v5"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(context.Background())

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(10*time.Second),
		distlimit.WithAlgorithm(fixedwindow.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	e := echo.New()
	e.Use(distlimitechov5.New(limiter))

	e.GET("/api/ping", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "success",
			"message": "pong from Echo v5!",
		})
	})

	log.Println("⚡ Echo v5 server running on http://localhost:1324")
	log.Fatal(e.Start(":1324"))
}
