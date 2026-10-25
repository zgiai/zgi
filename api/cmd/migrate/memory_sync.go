package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// runMemorySyncLegacy brings Agent memory from the legacy slot store into the
// relationship store before memory blocks are switched on (ADR 0013). It
// reports what it would change unless --apply is given.
func runMemorySyncLegacy(args []string) error {
	flags := flag.NewFlagSet("memory-sync-legacy", flag.ContinueOnError)
	apply := flags.Bool("apply", false, "apply the sync; without it the sync is planned and rolled back")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	db, err := openConfiguredDB()
	if err != nil {
		return err
	}
	report, err := relationshipmemory.SyncLegacyAgentMemory(context.Background(), db, *apply)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
