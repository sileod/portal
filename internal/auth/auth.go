package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func AgentToken(password string) string {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte("portal-agent-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}
