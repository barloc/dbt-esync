package qlik

import "testing"

func TestUnitNormalizeEngineHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"qlik.example.com", "qlik.example.com:4747"},
		{"qlik.example.com:4747", "qlik.example.com:4747"},
		{"wss://qlik.example.com/app/", "qlik.example.com:4747"},
		{"wss://qlik.example.com:443/app/", "qlik.example.com:443"},
		{"https://qlik.example.com/", "qlik.example.com:4747"},
		{"  WSS://Qlik.Example.Com/app/  ", "Qlik.Example.Com:4747"},
	}
	for _, tc := range cases {
		got := normalizeEngineHost(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeEngineHost(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
