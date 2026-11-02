//go:build windows

package main

import "errors"

// captureScreen is not written for Windows yet: paste an image copied
// with Win+Shift+S instead.
func captureScreen() ([]byte, error) {
	return nil, errors.New("screen capture is not supported on Windows yet; copy a screenshot with Win+Shift+S and paste it from the clipboard")
}
