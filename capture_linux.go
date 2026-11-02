//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// captureScreen lets the user pick a range of the screen with
// gnome-screenshot (X11 and GNOME) or slurp and grim (wlroots Wayland) and
// returns it as PNG. It blocks: call it from a goroutine.
func captureScreen() ([]byte, error) {
	dir, err := os.MkdirTemp("", "tcpmsg-capture")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "capture.png")
	if path, err := exec.LookPath("gnome-screenshot"); err == nil {
		if err := exec.Command(path, "-a", "-f", file).Run(); err != nil {
			return nil, errCaptureCancelled
		}
		return readCaptureFile(file)
	}
	grim, gerr := exec.LookPath("grim")
	slurp, serr := exec.LookPath("slurp")
	if gerr == nil && serr == nil {
		geom, err := exec.Command(slurp).Output()
		if err != nil || strings.TrimSpace(string(geom)) == "" {
			return nil, errCaptureCancelled
		}
		if err := exec.Command(grim, "-g", strings.TrimSpace(string(geom)), file).Run(); err != nil {
			return nil, err
		}
		return readCaptureFile(file)
	}
	return nil, errors.New("screen capture needs gnome-screenshot, or grim and slurp")
}
