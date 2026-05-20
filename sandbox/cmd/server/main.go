package main

import (
	"log"
	"net/http"

	"github.com/zgiai/zgi-sandbox/internal/app"
	"github.com/zgiai/zgi-sandbox/internal/config"
)

func main() {
	cfg := config.FromEnv()
	server, err := app.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("zgi-sandbox listening on http://127.0.0.1:%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
