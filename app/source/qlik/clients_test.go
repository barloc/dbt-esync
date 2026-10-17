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
)

func TestUnitStripBase64RemovesWhitespace(t *testing.T) {
	in := "YWJj\nZGVm\r\nZ2hp\t "
	got := stripBase64(in)
	if got != "YWJjZGVmZ2hp" {
		t.Fatalf("got %q", got)
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
