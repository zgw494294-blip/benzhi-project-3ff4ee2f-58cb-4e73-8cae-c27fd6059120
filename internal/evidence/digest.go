package evidence

import (
	"crypto/sha256"
	"encoding/hex"
)

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ShortDigest(value string) string { return Digest([]byte(value))[:20] }
