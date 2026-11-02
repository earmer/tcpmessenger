//go:build !darwin && !linux && !windows

package main

import "errors"

func captureScreen() ([]byte, error) {
	return nil, errors.New("screen capture is not supported on this system")
}
