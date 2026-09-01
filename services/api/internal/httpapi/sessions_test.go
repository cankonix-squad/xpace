package httpapi

import "testing"

func TestSessionDeviceName(t *testing.T) {
	tests := []struct{ agent, want string }{
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/537.36 Chrome/124.0 Safari/537.36", "Google Chrome on Mac"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1 Version/17.0 Mobile Safari/604.1", "Safari on iPhone"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Gecko Firefox/126.0", "Mozilla Firefox on Windows"},
		{"curl/8.0", "Browser on Unknown device"},
	}
	for _, test := range tests {
		if got := sessionDeviceName(test.agent); got != test.want {
			t.Errorf("sessionDeviceName(%q)=%q want %q", test.agent, got, test.want)
		}
	}
}
