package core

import (
	"crypto/sha256"
	"fmt"
)

var (
	fpColors = [16]string{"Cobalt", "Amber", "Jade", "Crimson", "Slate", "Ivory", "Teal", "Plum", "Rust", "Azure", "Olive", "Coral", "Indigo", "Sand", "Violet", "Onyx"}
	fpNouns  = [16]string{"Falcon", "Otter", "Maple", "Comet", "Harbor", "Lynx", "Cedar", "Ember", "Raven", "Tundra", "Sparrow", "Quartz", "Heron", "Basalt", "Willow", "Fox"}
)

// Fingerprint is the three-word name of a signing key, such as
// "Cobalt-Falcon-24": short enough to compare by eye.
func Fingerprint(key []byte) string {
	h := sha256.Sum256(key)
	return fmt.Sprintf("%s-%s-%02d", fpColors[h[0]&15], fpNouns[h[1]&15], h[2]%100)
}

// Identicon is the 5x5 pattern of a fingerprint, mirrored left to right,
// so that a changed key looks different at a glance.
func Identicon(fingerprint string) [5][5]bool {
	h := sha256.Sum256([]byte(fingerprint))
	var g [5][5]bool
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			on := h[4+y*3+x]&1 == 1
			g[y][x], g[y][4-x] = on, on
		}
	}
	return g
}
