//go:build linux

package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const processCgroupCleanupTimeout = time.Second

type processCgroupManager struct {
	root  string
	limit int
}

type executionCgroup struct {
	path string
}

func newProcessCgroupManager(root string, limit int) (*processCgroupManager, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return nil, errors.New("secure runtime cgroup root must be an absolute path")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("stat secure runtime cgroup root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("secure runtime cgroup root must be a directory and not a symlink")
	}
	subtreeControl, err := os.ReadFile(filepath.Join(root, "cgroup.subtree_control"))
	if err != nil {
		return nil, fmt.Errorf("read secure runtime cgroup controllers: %w", err)
	}
	if !hasSpaceSeparatedValue(string(subtreeControl), "pids") {
		return nil, errors.New("secure runtime cgroup root does not delegate the pids controller")
	}
	return &processCgroupManager{root: root, limit: limit}, nil
}

func (m *processCgroupManager) Create() (*executionCgroup, error) {
	path := filepath.Join(m.root, "execution-"+token())
	if err := os.Mkdir(path, 0o700); err != nil {
		return nil, fmt.Errorf("create secure runtime process cgroup: %w", err)
	}
	if err := os.WriteFile(filepath.Join(path, "pids.max"), []byte(strconv.Itoa(m.limit)), 0o600); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("set secure runtime process limit: %w", err)
	}
	return &executionCgroup{path: path}, nil
}

func (g *executionCgroup) Close() error {
	killPath := filepath.Join(g.path, "cgroup.kill")
	if _, err := os.Stat(killPath); err == nil {
		if err := os.WriteFile(killPath, []byte("1"), 0o600); err != nil {
			return fmt.Errorf("kill remaining secure runtime cgroup processes: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat secure runtime cgroup kill control: %w", err)
	}

	deadline := time.Now().Add(processCgroupCleanupTimeout)
	for {
		populated, err := readCgroupPopulated(filepath.Join(g.path, "cgroup.events"))
		if err != nil {
			return err
		}
		if !populated {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("secure runtime process cgroup remained populated after termination")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.Remove(g.path); err != nil {
		return fmt.Errorf("remove secure runtime process cgroup: %w", err)
	}
	return nil
}

func readCgroupPopulated(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read secure runtime cgroup events: %w", err)
	}
	fields := strings.Fields(string(data))
	for index := 0; index+1 < len(fields); index += 2 {
		if fields[index] != "populated" {
			continue
		}
		switch fields[index+1] {
		case "0":
			return false, nil
		case "1":
			return true, nil
		default:
			return false, fmt.Errorf("invalid secure runtime cgroup populated value %q", fields[index+1])
		}
	}
	return false, errors.New("secure runtime cgroup events omitted populated state")
}

func hasSpaceSeparatedValue(value, want string) bool {
	for _, field := range strings.Fields(value) {
		if field == want {
			return true
		}
	}
	return false
}
