//go:build !windows && !bindings

package main

import (
	"log"
	"os/exec"
)

func showNativeError(message string)  { log.Print(message) }
func hideCleanupWindow(cmd *exec.Cmd) {}
