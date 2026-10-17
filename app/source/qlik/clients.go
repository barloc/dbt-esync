package qlik

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qlik-oss/enigma-go/v4"
	"golang.org/x/crypto/pkcs12"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/parse"
)

// defaultClients implements Catalog, ScriptFetcher, and TagFetcher via QRS + Engine.
type defaultClients struct {
	cfg        config.QlikConfig
	httpClient *http.Client
	tlsConfig  *tls.Config
	qlikUser   string
}

func newDefaultClients(cfg config.QlikConfig, httpClient *http.Client) (*defaultClients, error) {
	if cfg.QRC.Host == "" || cfg.QRC.CertB64 == "" {
		return nil, fmt.Errorf("qlik: default client requires qrc.host and qrc.cert_b64 (or inject Catalog/Scripts)")
	}
	tlsCfg, err := tlsFromPKCS12(cfg.QRC.CertB64, cfg.QRC.Password)
	if err != nil {
		return nil, err
	}
	dir := cfg.QRC.UserDirectory
	uid := cfg.QRC.UserID
	if dir == "" || uid == "" {
		// Fallback: qlik.user as "DIRECTORY\\userid" (Windows-style).
		dir, uid = splitSenseUser(cfg.User)
	}
	if dir == "" || uid == "" {
		return nil, fmt.Errorf("qlik: qrc.user_directory/user_id (or qlik.user as DIR\\\\id) required for default client")
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	if httpClient == nil {
		httpClient = &http.Client{Transport: transport}
	} else {
		// Keep caller timeouts/etc., but always attach Sense client cert.
		cloned := *httpClient
		if base, ok := httpClient.Transport.(*http.Transport); ok && base != nil {
			t := base.Clone()
			t.TLSClientConfig = tlsCfg
			cloned.Transport = t
		} else {
			cloned.Transport = transport
		}
		httpClient = &cloned
	}
	return &defaultClients{
		cfg:        cfg,
		httpClient: httpClient,
		tlsConfig:  tlsCfg,
		qlikUser:   fmt.Sprintf("UserDirectory=%s; UserId=%s", dir, uid),
	}, nil
}

func splitSenseUser(user string) (directory, id string) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", ""
	}
	if i := strings.IndexAny(user, `/\`); i >= 0 {
		return user[:i], user[i+1:]
	}
	return "", user
}

func tlsFromPKCS12(certB64, password string) (*tls.Config, error) {
	raw, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return nil, fmt.Errorf("qlik: cert_b64: %w", err)
	}
	key, cert, err := pkcs12.Decode(raw, password)
	if err != nil {
		return nil, fmt.Errorf("qlik: pkcs12: %w", err)
	}
	tlsCert := tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
		// ponytail: Sense lab certs are often self-signed; InsecureSkipVerify is a known ceiling.
		// Upgrade: pin CA via optional qrc.ca_b64 when available.
		InsecureSkipVerify: true, //nolint:gosec // Sense client-cert labs rarely ship a public CA
	}, nil
}

type qrsApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Stream *struct {
		Name string `json:"name"`
	} `json:"stream"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

func (c *defaultClients) ListApps(ctx context.Context) ([]App, error) {
	body, err := c.qrsGET(ctx, "/qrs/app/full")
	if err != nil {
		return nil, err
	}
	var apps []qrsApp
	if err := json.Unmarshal(body, &apps); err != nil {
		return nil, fmt.Errorf("qlik: decode apps: %w", err)
	}
	out := make([]App, 0, len(apps))
	for _, a := range apps {
		stream := ""
		if a.Stream != nil {
			stream = a.Stream.Name
		}
		out = append(out, App{ID: a.ID, Name: a.Name, Stream: stream})
	}
	return out, nil
}

func (c *defaultClients) AppTags(ctx context.Context, appID string) ([]parse.Tag, error) {
	body, err := c.qrsGET(ctx, "/qrs/app/"+appID)
	if err != nil {
		return nil, err
	}
	var a qrsApp
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("qlik: decode app: %w", err)
	}
	out := make([]parse.Tag, 0, len(a.Tags))
	for _, t := range a.Tags {
		if t.Name != "" {
			out = append(out, parse.Tag{Name: t.Name})
		}
	}
	return out, nil
}

func (c *defaultClients) qrsGET(ctx context.Context, path string) ([]byte, error) {
	xrf := "0123456789abcdef"
	u := strings.TrimRight(c.cfg.QRC.Host, "/")
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+path+"?xrfkey="+xrf, nil)
	if err != nil {
		return nil, fmt.Errorf("qlik: qrs request: %w", err)
	}
	req.Header.Set("X-Qlik-Xrfkey", xrf)
	req.Header.Set("X-Qlik-User", c.qlikUser)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qlik: qrs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qlik: qrs read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qlik: qrs status %d", resp.StatusCode)
	}
	return body, nil
}

func (c *defaultClients) LoadScript(ctx context.Context, appID string) (string, error) {
	host := c.cfg.Host
	if !strings.Contains(host, ":") {
		host = host + ":4747"
	}
	engineURL := "wss://" + host + "/app/"
	headers := http.Header{}
	headers.Set("X-Qlik-User", c.qlikUser)

	dialer := enigma.Dialer{TLSClientConfig: c.tlsConfig}
	global, err := dialer.Dial(ctx, engineURL, headers)
	if err != nil {
		return "", fmt.Errorf("qlik: engine dial: %w", err)
	}
	defer global.DisconnectFromServer()

	doc, err := global.OpenDoc(ctx, appID, "", "", "", true)
	if err != nil {
		return "", fmt.Errorf("qlik: open doc: %w", err)
	}
	script, err := doc.GetScript(ctx)
	if err != nil {
		return "", fmt.Errorf("qlik: get script: %w", err)
	}
	return script, nil
}
