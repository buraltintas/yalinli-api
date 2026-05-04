package auth

import (
	"crypto/rand"
	"fmt"
)

func GenerateNumericCode() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	n = n%900000 + 100000
	return fmt.Sprintf("%06d", n), nil
}
