package random

import (
	mathrand "math/rand"
	"strings"
	"time"
)

func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	seededRand := mathrand.New(mathrand.NewSource(time.Now().UnixNano() + int64(mathrand.Intn(9999))))
	var result strings.Builder
	for i := 0; i < length; i++ {
		randomIndex := seededRand.Intn(len(charset))
		result.WriteString(string(charset[randomIndex]))
	}
	return result.String()
}
