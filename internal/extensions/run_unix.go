// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package extensions

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// relaySignals forwards SIGTERM and SIGHUP to process until stop is called.
func relaySignals(process *os.Process) (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				_ = process.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func exitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}

// checkAccess refuses a file that another user, other than root, owns, or that anyone can write
// to, and a directory anyone can write to unless its sticky bit keeps others from replacing the
// user's own file in it.
func checkAccess(path string, file, dir fs.FileInfo) error {
	owner := os.Getuid()
	if stat, ok := file.Sys().(*syscall.Stat_t); ok {
		owner = int(stat.Uid)
	}
	switch {
	case owner != os.Getuid() && owner != 0:
		return fmt.Errorf("%s is owned by another user", path)
	case file.Mode().Perm()&0o002 != 0:
		return fmt.Errorf("%s is writable by every user", path)
	case dir.Mode().Perm()&0o002 != 0 && (dir.Mode()&fs.ModeSticky == 0 || owner != os.Getuid()):
		return fmt.Errorf("%s is writable by every user", filepath.Dir(path))
	}
	return nil
}
