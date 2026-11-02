package store

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	hashPrefix = "pbkdf2-sha256$"
	hashIter   = 100_000
	hashLen    = 32
)

// HashPassword returns a salted PBKDF2-SHA256 hash of pw in the form
// "pbkdf2-sha256$<iter>$<salt>$<key>". An empty pw hashes to "".
func HashPassword(pw string) string {
	if pw == "" {
		return ""
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	key, err := pbkdf2.Key(sha256.New, pw, salt, hashIter, hashLen)
	if err != nil {
		panic(err)
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s%d$%s$%s", hashPrefix, hashIter, enc.EncodeToString(salt), enc.EncodeToString(key))
}

// IsHashed reports whether s looks like a value HashPassword returned.
func IsHashed(s string) bool { return strings.HasPrefix(s, hashPrefix) }

// CheckPassword reports whether pw matches hash. An empty hash means no
// password is set, and only the empty pw matches it.
func CheckPassword(hash, pw string) bool {
	if hash == "" {
		return pw == ""
	}
	if !IsHashed(hash) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(hash, hashPrefix), "$")
	if len(parts) != 3 {
		return false
	}
	iter, err := strconv.Atoi(parts[0])
	if err != nil || iter <= 0 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[1])
	want, err2 := enc.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
