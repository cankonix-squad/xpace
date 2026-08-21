package httpapi

import "testing"

func TestValidLocale(t *testing.T) {
	for _, locale := range []string{"en", "en-ID", "id-ID"} {
		if !validLocale(locale) {
			t.Errorf("validLocale(%q) = false", locale)
		}
	}
	for _, locale := range []string{"", "e", "en_ID", "en ID", "123"} {
		if validLocale(locale) {
			t.Errorf("validLocale(%q) = true", locale)
		}
	}
}
