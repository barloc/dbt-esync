package qlik

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const proxyTimeout = 10 * time.Minute

// proxyAppBase normalizes the WSS proxy URL to end with /app/ (Python qlik_host).
func proxyAppBase(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	lower := strings.ToLower(host)
	if !strings.HasPrefix(lower, "ws://") && !strings.HasPrefix(lower, "wss://") {
		host = "wss://" + host
		lower = strings.ToLower(host)
	}
	if !strings.HasSuffix(host, "/") {
		host += "/"
	}
	if strings.HasSuffix(lower, "/app/") || strings.HasSuffix(strings.ToLower(host), "/app/") {
		return host
	}
	if strings.HasSuffix(strings.ToLower(host), "/app") {
		return host + "/"
	}
	return host + "app/"
}

// formsUsername matches Python forms_username: Vault/Airflow often store + for \.
func formsUsername(login string) string {
	return strings.ReplaceAll(strings.TrimSpace(login), "+", `\`)
}

func ticketFromHTTP(loc, finalURL string) string {
	for _, candidate := range []string{finalURL, loc} {
		if i := strings.Index(candidate, "qlikTicket="); i >= 0 {
			t := candidate[i+len("qlikTicket="):]
			if j := strings.IndexAny(t, "&#"); j >= 0 {
				t = t[:j]
			}
			return t
		}
	}
	if u, err := url.Parse(loc); err == nil {
		if t := u.Query().Get("qlikTicket"); t != "" {
			return t
		}
	}
	return ""
}

// ensureProxyAuth obtains qlikTicket + session cookie once (Python _get_ticket / _get_cookies).
func (c *defaultClients) ensureProxyAuth(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cookie != "" {
		return nil
	}
	base := proxyAppBase(c.cfg.Host)
	if base == "" {
		return fmt.Errorf("qlik: host is required (WSS proxy URL, e.g. wss://qlik.example/app/)")
	}
	user := formsUsername(c.cfg.User)
	if user == "" || c.cfg.Password == "" {
		return fmt.Errorf("qlik: user and password are required for proxy ticket auth")
	}

	slog.Info("qlik proxy ticket start", "host", base)
	ticket, err := fetchQlikTicket(ctx, base, user, c.cfg.Password, c.plainHTTP)
	if err != nil {
		return err
	}
	cookie, err := fetchQlikSessionCookie(ctx, base, ticket)
	if err != nil {
		return err
	}
	c.cookie = cookie
	slog.Info("qlik proxy ticket ok")
	return nil
}

func fetchQlikTicket(ctx context.Context, appBase, user, password string, httpClient *http.Client) (string, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: proxyTimeout,
		TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, _, err := dialer.DialContext(ctx, appBase, nil)
	if err != nil {
		return "", fmt.Errorf("qlik: proxy dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(proxyTimeout))
	_ = conn.SetWriteDeadline(time.Now().Add(proxyTimeout))

	if err := conn.WriteJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"handle":  -1,
		"method":  "GetDocList",
		"params":  []any{},
	}); err != nil {
		return "", fmt.Errorf("qlik: proxy GetDocList: %w", err)
	}

	var msg struct {
		Method string `json:"method"`
		Params struct {
			MustAuthenticate bool   `json:"mustAuthenticate"`
			LoginURI         string `json:"loginUri"`
		} `json:"params"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		return "", fmt.Errorf("qlik: proxy auth read: %w", err)
	}
	if msg.Method != "OnAuthenticationInformation" || !msg.Params.MustAuthenticate || msg.Params.LoginURI == "" {
		return "", fmt.Errorf("qlik: proxy did not require authentication (method=%q)", msg.Method)
	}

	if httpClient == nil {
		httpClient = newPlainHTTP()
	}
	form := url.Values{"username": {user}, "pwd": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, msg.Params.LoginURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("qlik: proxy login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("qlik: proxy login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	ticket := ticketFromHTTP(resp.Header.Get("Location"), resp.Request.URL.String())
	if ticket == "" {
		return "", fmt.Errorf("qlik: proxy login: no qlikTicket (status=%d url=%q body=%q)",
			resp.StatusCode, resp.Header.Get("Location"), truncate(body, 200))
	}
	return ticket, nil
}

func fetchQlikSessionCookie(ctx context.Context, appBase, ticket string) (string, error) {
	u := appBase + "engineData?qlikTicket=" + url.QueryEscape(ticket)
	dialer := websocket.Dialer{
		HandshakeTimeout: proxyTimeout,
		TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec
	}
	conn, resp, err := dialer.DialContext(ctx, u, nil)
	if err != nil {
		return "", fmt.Errorf("qlik: proxy session dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(proxyTimeout))
	_ = conn.SetWriteDeadline(time.Now().Add(proxyTimeout))

	if err := conn.WriteJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"handle":  -1,
		"method":  "GetDocList",
		"params":  []any{},
	}); err != nil {
		return "", fmt.Errorf("qlik: proxy session GetDocList: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := conn.ReadJSON(&raw); err != nil {
		return "", fmt.Errorf("qlik: proxy session read: %w", err)
	}
	if methodRaw, ok := raw["method"]; ok {
		var method string
		_ = json.Unmarshal(methodRaw, &method)
		if method == "OnMaxParallelSessionsExceeded" {
			return "", fmt.Errorf("qlik: MaxParallelSessionsExceeded")
		}
	}

	if resp == nil {
		return "", fmt.Errorf("qlik: proxy session: no handshake response")
	}
	cookie := strings.Join(resp.Header.Values("Set-Cookie"), "; ")
	if cookie == "" {
		return "", fmt.Errorf("qlik: proxy session: empty Set-Cookie")
	}
	return cookie, nil
}

func newPlainHTTP() *http.Client {
	return &http.Client{
		Timeout: proxyTimeout,
		// Do not follow redirects — ticket is in Location (Python reads response.url after follow;
		// with CheckRedirect we take Location explicitly). Also skip env HTTP(S)_PROXY.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, //nolint:gosec
			},
		},
	}
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
