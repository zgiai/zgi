//go:build !linux

package main

import (
	"context"
	"errors"
	"log"
)

func run(context.Context, *log.Logger) error {
	return errors.New("zgi Agent Runtime worker requires Linux")
}
