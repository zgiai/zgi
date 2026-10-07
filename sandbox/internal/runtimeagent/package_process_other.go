//go:build !linux

package runtimeagent

import (
	"os"
	"os/exec"
)

func openPackageLedgerFile(root *os.Root) (*os.File, error) { return root.Open(packageLedgerName) }

func configurePackageProcess(command *exec.Cmd) {}
