//go:build windows

package sshlib

import (
	"os"
)

func notifyWinch(ch chan os.Signal) {
	// SIGWINCH does not exist on Windows
}
