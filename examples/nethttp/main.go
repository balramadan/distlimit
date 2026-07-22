package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/slidinglog"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitnethttp "github.com/balramadan/distlimit/middleware/nethttp"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(5),
		distlimit.WithWindow(10*time.Second),
		distlimit.WithAlgorithm(slidinglog.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"status":"success","message":"pong from net/http!"}`)
	})

	// Wrap handler dengan distlimit middleware
	handler := distlimitnethttp.New(limiter)(mux)

	log.Println("⚡ Standard net/http server running on http://localhost:8081")
	log.Fatal(http.ListenAndServe(":8081", handler))
}
