//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	// Get the handle to the standard output
	stdout := windows.Handle(os.Stdout.Fd())
	var originalMode uint32

	// Read current console mode
	err := windows.GetConsoleMode(stdout, &originalMode)
	if err == nil {
		// Activate the Virtual Terminal Processing for ANSI-Colors
		windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
