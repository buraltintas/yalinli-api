package validation

import (
	"errors"
	"net/mail"
	"strings"
)

func NormalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func ValidateEmail(v string) error {
	_, err := mail.ParseAddress(v)
	if err != nil {
		return errors.New("invalid email")
	}
	return nil
}
func ValidLang(v string) bool { return v == "tr" || v == "en" }
func Trim(v string) string    { return strings.TrimSpace(v) }
