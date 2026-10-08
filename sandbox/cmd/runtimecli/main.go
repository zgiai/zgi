package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/platformcli"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--help" {
		os.Exit(platformcli.Run(context.Background(), nil, os.Args[1:], os.Stdout, os.Stderr))
	}
	client, err := platformcli.NewClient(os.Getenv("ZGI_PLATFORM_SOCKET"), os.Getenv("ZGI_PLATFORM_JOB_ID"), os.Getenv("ZGI_PLATFORM_TOKEN"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "platform CLI is unavailable for this Job")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	exit := platformcli.Run(ctx, client, os.Args[1:], os.Stdout, os.Stderr)
	client.Close()
	stop()
	cancel()
	os.Exit(exit)
}
