package auth

import "testing"

func TestGenerateNumericCode(t *testing.T) {
	code, err := GenerateNumericCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6 digits, got %s", code)
	}
}
