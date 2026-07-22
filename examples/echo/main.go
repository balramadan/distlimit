package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/leakybucket"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitecho "github.com/balramadan/distlimit/middleware/echo"
	"github.com/labstack/echo/v4"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer memDriver.Close(context.Background())

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(10*time.Second),
		distlimit.WithAlgorithm(leakybucket.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	e := echo.New()
	e.Use(distlimitecho.New(limiter))

	e.GET("/api/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "success",
			"message": "pong from Echo v4!",
		})
	})

	log.Println("⚡ Echo v4 server running on http://localhost:1323")
	log.Fatal(e.Start(":1323"))
}
