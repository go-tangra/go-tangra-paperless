package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

var downloadSecret []byte

func init() {
	secret := os.Getenv("DOWNLOAD_TOKEN_SECRET")
	if secret == "" {
		secret = "go-tangra-paperless-download-secret"
	}
	downloadSecret = []byte(secret)
}

// GenerateDownloadToken creates an HMAC token for time-limited document downloads
func GenerateDownloadToken(requestID string, expiresAt time.Time) string {
	mac := hmac.New(sha256.New, downloadSecret)
	mac.Write([]byte(fmt.Sprintf("%s:%d", requestID, expiresAt.Unix())))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateDownloadToken checks the HMAC token and expiration
func ValidateDownloadToken(requestID string, token string, expiresUnix int64) bool {
	if time.Now().Unix() > expiresUnix {
		return false
	}
	expected := GenerateDownloadToken(requestID, time.Unix(expiresUnix, 0))
	return hmac.Equal([]byte(token), []byte(expected))
}
