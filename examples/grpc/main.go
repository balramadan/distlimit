package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/balramadan/distlimit"
	"github.com/balramadan/distlimit/algorithm/slidingcounter"
	"github.com/balramadan/distlimit/driver/memory"
	distlimitgrpc "github.com/balramadan/distlimit/middleware/grpc"
	"google.golang.org/grpc"
)

func main() {
	memDriver := memory.New(5 * time.Minute)
	defer func() { _ = memDriver.Close(context.Background()) }()

	limiter, err := distlimit.New(
		memDriver,
		distlimit.WithLimit(100),
		distlimit.WithWindow(1*time.Minute),
		distlimit.WithAlgorithm(slidingcounter.New()),
	)
	if err != nil {
		log.Fatalf("Failed to initialize limiter: %v", err)
	}

	// Pasang gRPC Unary Interceptor
	server := grpc.NewServer(
		grpc.UnaryInterceptor(distlimitgrpc.UnaryServerInterceptor(limiter)),
		grpc.StreamInterceptor(distlimitgrpc.StreamServerInterceptor(limiter)),
	)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on :50051: %v", err)
	}

	log.Println("⚡ gRPC server running on :50051 with distlimit interceptors")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
