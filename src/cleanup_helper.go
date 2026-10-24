//go:build !bindings

package main

import (
	"context"
	"os"
	"os/exec"
	"tdl-ui/internal/applock"
	"tdl-ui/internal/config"
	"time"
)

func runCleanupHelper() bool {
	if len(os.Args) < 2 || os.Args[1] != "--complete-confirmed-cleanup" {
		return false
	}
	if len(os.Args) != 3 || !config.HasPendingCleanup(os.Args[2]) {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	guard, err := applock.Acquire(ctx, os.Args[2])
	if err != nil {
		return true
	}
	defer guard.Close()
	for {
		if err := config.RunPendingCleanup(os.Args[2]); err == nil {
			return true
		}
		select {
		case <-ctx.Done():
			return true
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func startCleanupHelper(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--complete-confirmed-cleanup", dir)
	hideCleanupWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
