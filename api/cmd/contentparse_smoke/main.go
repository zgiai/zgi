package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func main() {
	var (
		filePath = flag.String("file", "", "Path to the file to parse")
		engine   = flag.String("engine", "local", "Engine hint: local|mineru|reducto|vlm")
		intent   = flag.String("intent", "preview", "Intent: preview|dataset_index|chat_context")
		profile  = flag.String("profile", "fast_preview", "Profile: default|fast_preview|layout_first|text_first|dataset_index")
	)
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "missing required --file")
		os.Exit(2)
	}

	data, err := os.ReadFile(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read file: %v\n", err)
		os.Exit(1)
	}

	module := contentparse.NewModule()
	artifact, err := module.Service.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   filepath.Base(*filePath),
		Data:       data,
		Intent:     contracts.ParseIntent(*intent),
		Profile:    contracts.ParseProfile(*profile),
		EngineHint: contracts.ParseEngine(*engine),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse failed: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal artifact: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
