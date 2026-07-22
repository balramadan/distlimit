package main

import (
	"context"
	"log"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitfiber "github.com/balramadan/distlimit/middleware/fiber"
	"github.com/gofiber/fiber/v3"
)

func main() {
	// 1. Driver
	memDriver := memory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	// 2. Limiter dengan Sliding Counter Algorithm (5 req / 10s)
	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(10*time.Second),
		distlimit.WithAlgorithm(slidingcounter.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	app := fiber.New()
	app.Use(distlimitfiber.New(limiter))

	app.Get("/api/ping", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "pong from Fiber v3!",
		})
	})

	log.Println("⚡ Fiber v3 server running on http://localhost:3000")
	log.Fatal(app.Listen(":3000"))
}
