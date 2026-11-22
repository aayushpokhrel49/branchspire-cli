// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package extensions

import (
	"io/fs"
	"os"
	"os/signal"
)

// relaySignals keeps bh alive while the extension handles Ctrl-C, which Windows delivers to every
// process of the console.
func relaySignals(*os.Process) (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	return func() { signal.Stop(signals) }
}

func exitCode(state *os.ProcessState) int { return state.ExitCode() }

// checkAccess has nothing to check: Windows guards files with ACLs, not mode bits.
func checkAccess(string, fs.FileInfo, fs.FileInfo) error { return nil }
