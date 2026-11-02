//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// captureScreen lets the user pick a range of the screen with macOS's own
// screencapture tool and returns it as PNG. It blocks: call it from a
// goroutine. errCaptureCancelled means the user pressed Escape.
func captureScreen() ([]byte, error) {
	dir, err := os.MkdirTemp("", "tcpmsg-capture")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "capture.png")
	// -i: interactive range (Space toggles window mode), -x: no sound.
	if err := exec.Command("/usr/sbin/screencapture", "-i", "-x", file).Run(); err != nil {
		return nil, errCaptureCancelled
	}
	return readCaptureFile(file)
}
