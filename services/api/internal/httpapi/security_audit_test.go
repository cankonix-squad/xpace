package httpapi

import "testing"

func TestShortIdentityHashIsStableAndNonReflective(t *testing.T) {
	first := shortIdentityHash(" Admin@Example.com ")
	second := shortIdentityHash("admin@example.com")
	if first != second || len(first) != 16 {
		t.Fatalf("unexpected identity hash: %q %q", first, second)
	}
	if first == "admin@example.com" {
		t.Fatal("audit metadata must not reflect login identity")
	}
}
