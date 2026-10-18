//go:build !linux

package runtimeagent

import (
	"context"
	"errors"
	"os"
)

func prepareJobFileOwnership(*os.File, uint32, uint32) error { return nil }

func prepareWorkspaceDirectoryOwnership(*os.Root, string, uint32, uint32) error { return nil }

func prepareSessionPythonEnvironment(context.Context, string, uint32, uint32) error {
	return errors.New("session Python package installation requires Linux")
}
