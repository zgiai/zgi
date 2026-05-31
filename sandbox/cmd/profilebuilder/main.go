package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zgiai/zgi-sandbox/internal/profilebuilder"
)

func main() {
	var opts profilebuilder.Options
	flag.StringVar(&opts.ProfileName, "profile", "", "profile source name to build")
	flag.StringVar(&opts.SourceDir, "source-dir", "profiles", "profile source catalog directory")
	flag.StringVar(&opts.OutputDir, "output-dir", ".profile-build/profiles", "profile artifact output directory")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "validate and print build steps without writing output")
	flag.BoolVar(&opts.Force, "force", false, "replace an existing output profile directory")
	flag.Parse()

	result, err := profilebuilder.Build(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "profile build failed: %v\n", err)
		os.Exit(1)
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(raw))
}
