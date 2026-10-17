package qlik

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/barloc/dbt-esync/app/config"
)

func TestUnitStripBase64RemovesWhitespace(t *testing.T) {
	in := "YWJj\nZGVm\r\nZ2hp\t "
	got := stripBase64(in)
	if got != "YWJjZGVmZ2hp" {
		t.Fatalf("got %q", got)
	}
}

func TestUnitStripBase64RemovesLiteralBackslashN(t *testing.T) {
	// Same shape as Python: cert.replace('\\n', '')
	in := `YWJj\nZGVm\r\nZ2hp`
	got := stripBase64(in)
	if got != "YWJjZGVmZ2hp" {
		t.Fatalf("got %q", got)
	}
	// Corrupt if only real newlines stripped: backslash dropped, lone 'n' kept.
	if stripBase64(`ab\ncd`) != "abcd" {
		t.Fatalf("literal \\n must remove both chars, got %q", stripBase64(`ab\ncd`))
	}
}

func TestUnitIsPEM(t *testing.T) {
	if !isPEM([]byte("-----BEGIN CERTIFICATE-----\nMIIB\n")) {
		t.Fatal("expected PEM")
	}
	if isPEM([]byte{0x30, 0x82, 0x01}) {
		t.Fatal("binary should not be PEM")
	}
}

func TestUnitTLSFromClientMaterialPEM(t *testing.T) {
	certPEM, keyPEM := mustTestPEMs(t)
	certB64 := base64.StdEncoding.EncodeToString(certPEM)
	// Simulate Airflow var with newlines in the middle of base64.
	var wrapped strings.Builder
	for i, c := range certB64 {
		if i > 0 && i%64 == 0 {
			wrapped.WriteByte('\n')
		}
		wrapped.WriteByte(byte(c))
	}
	keyB64 := base64.StdEncoding.EncodeToString(keyPEM)

	cfg, err := tlsFromClientMaterial(wrapped.String(), keyB64)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("certs: %d", len(cfg.Certificates))
	}
}

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

func TestUnitEngineDialHostPrefersQRC(t *testing.T) {
	t.Parallel()
	cfg := config.QlikConfig{
		Host: "wss://proxy.example.com/app/",
		QRC:  config.QRCConfig{Host: "https://sense.example.com/"},
	}
	got := engineDialHost(cfg)
	if got != "sense.example.com:4747" {
		t.Fatalf("got %q", got)
	}
	cfg.QRC.Host = ""
	got = engineDialHost(cfg)
	if got != "proxy.example.com:4747" {
		t.Fatalf("fallback got %q", got)
	}
}

func mustTestPEMs(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
