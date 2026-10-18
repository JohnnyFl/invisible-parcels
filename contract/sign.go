package contract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign returns hex(HMAC-SHA256(body, secret)) for the X-Signature header.
func Sign(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func Verify(body []byte, secret, sig string) bool {
	return hmac.Equal([]byte(Sign(body, secret)), []byte(sig))
}
