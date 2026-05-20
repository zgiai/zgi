package main

import (
	"context"
	"log"
	"time"

	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to Console gRPC server
	conn, err := grpc.Dial("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewChannelServiceClient(conn)

	// Test 1: List Official Channels (unary RPC)
	log.Println("[Test 1] Calling ListOfficialChannels...")
	listResp, err := client.ListOfficialChannels(context.Background(), &pb.ListOfficialChannelsRequest{})
	if err != nil {
		log.Fatalf("ListOfficialChannels failed: %v", err)
	}
	log.Printf("[Test 1] ✅ Received %d channels\n", len(listResp.Channels))
	for i, ch := range listResp.Channels {
		log.Printf("  %d. %s (%s) - %s", i+1, ch.Name, ch.Provider, ch.Protocol)
	}

	// Test 2: Watch Channels (streaming RPC)
	log.Println("\n[Test 2] Starting WatchChannels stream...")
	stream, err := client.WatchChannels(context.Background(), &pb.WatchChannelsRequest{})
	if err != nil {
		log.Fatalf("WatchChannels failed: %v", err)
	}

	// Receive events for 10 seconds
	go func() {
		for {
			event, err := stream.Recv()
			if err != nil {
				log.Printf("[Stream] Error: %v", err)
				return
			}

			switch event.Type {
			case pb.ChannelEvent_SNAPSHOT:
				log.Printf("[Stream] SNAPSHOT: %s (v%d)", event.Channel.Name, event.Version)
			case pb.ChannelEvent_CREATED:
				log.Printf("[Stream] CREATED: %s (v%d)", event.Channel.Name, event.Version)
			case pb.ChannelEvent_UPDATED:
				log.Printf("[Stream] UPDATED: %s (v%d)", event.Channel.Name, event.Version)
			case pb.ChannelEvent_ENABLED:
				log.Printf("[Stream] ENABLED: %s (v%d)", event.Channel.Name, event.Version)
			case pb.ChannelEvent_DISABLED:
				log.Printf("[Stream] DISABLED: %s (v%d)", event.Channel.Name, event.Version)
			}
		}
	}()

	// Keep client running
	log.Println("[Test 2] Listening for events (will run for 60 seconds)...")
	log.Println("        Perform CRUD operations in Console to see real-time events!")
	time.Sleep(60 * time.Second)

	log.Println("\n✅ All tests completed successfully!")
}
