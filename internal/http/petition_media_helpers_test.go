package http

import (
	"testing"
	"time"
)

func TestMaskFullName(t *testing.T) {
	got := maskFullName("Burak Altintas", "tr")
	if got == "Burak Altintas" || got == "" {
		t.Fatalf("unexpected mask: %q", got)
	}
	got2 := maskFullName("X", "en")
	if got2 != "Masked User" {
		t.Fatalf("expected Masked User, got %q", got2)
	}
}

func TestLegalTexts(t *testing.T) {
	if electronicApprovalText("tr") != "yalinli.org üzerinden elektronik olarak onaylanmıştır." {
		t.Fatal("tr approval text mismatch")
	}
	if electronicApprovalText("en") != "Electronically approved through yalinli.org." {
		t.Fatal("en approval text mismatch")
	}
	if wetSignatureLabel("tr") != "Islak İmza" || wetSignatureLabel("en") != "Wet Signature" {
		t.Fatal("wet signature labels mismatch")
	}
	if legalNote("tr") == "" || legalNote("en") == "" {
		t.Fatal("legal note empty")
	}
}

func TestMakeSignatureHashDeterministic(t *testing.T) {
	ts := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	h1 := makeSignatureHash("secret", "p1", "u1", ts, "a@example.com", "v1")
	h2 := makeSignatureHash("secret", "p1", "u1", ts, "a@example.com", "v1")
	h3 := makeSignatureHash("secret", "p1", "u2", ts, "a@example.com", "v1")
	if h1 == "" || h1 != h2 || h1 == h3 {
		t.Fatal("signature hash behavior mismatch")
	}
}
