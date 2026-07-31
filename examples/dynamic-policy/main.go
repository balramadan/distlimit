package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/tokenbucket"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitnethttp "github.com/balramadan/distlimit/middleware/nethttp"
)

// DynamicTierResolver mensimulasikan penentuan policy berdasarkan User Tier (VIP vs Standard).
type DynamicTierResolver struct{}

func (r *DynamicTierResolver) ResolvePolicy(ctx context.Context, key string) (distlimit.Policy, bool) {
	// Jika request berasal dari VIP User (misal key ber-prefix "vip:"), berikan kuota tinggi (100 req/min)
	if strings.HasPrefix(key, "vip:") {
		return distlimit.Policy{
			Limit:  100,
			Window: 1 * time.Minute,
		}, true
	}

	// Untuk pengguna umum, gunakan default policy
	return distlimit.Policy{}, false
}

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	// Limiter dengan default limit 2 req/min dan dynamic policy resolver
	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(2),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(tokenbucket.New()),
		distlimit.WithPolicyResolver(&DynamicTierResolver{}),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	mux := http.NewServeMux()

	// Handler untuk mengubah global policy secara runtime tanpa restart server
	mux.HandleFunc("/admin/update-policy", func(w http.ResponseWriter, r *http.Request) {
		limiter.UpdatePolicy(50, 1*time.Minute)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","message":"Global policy updated to 50 requests/minute"}`))
	})

	// Endpoint API publik yang diproteksi rate limit
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userType := r.Header.Get("X-User-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","message":"Access granted for user type: %s"}`, userType)
	})

	// Setup Rate Limiting Middleware dengan Custom Key Extraction
	rateLimitedMiddleware := distlimitnethttp.New(
		limiter,
		distlimitnethttp.WithKeyFunc(func(ctx context.Context) string {
			// Membaca header X-User-ID dari request context jika ada
			return "user_generic_key"
		}),
	)

	mux.Handle("/api/resource", rateLimitedMiddleware(apiHandler))

	log.Println("⚡ Dynamic Policy & Runtime Reload Server running on http://localhost:8082")
	log.Println("💡 Try GET http://localhost:8082/api/resource")
	log.Println("💡 Try POST/GET http://localhost:8082/admin/update-policy")
	log.Fatal(http.ListenAndServe(":8082", mux))
}
