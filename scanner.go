package main

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ScanResult is the single source of truth for one URL inspection.
// Every feature layer reads this struct — do not change field names
// without migrating the scans table and all dependent handlers.
type ScanResult struct {
	URL        string            `json:"url"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	BodyHash   string            `json:"body_hash"`
	BodySize   int               `json:"body_size"`
	TLSIssuer  string            `json:"tls_issuer"`
	TLSExpiry  time.Time         `json:"tls_expiry"`
	Server     string            `json:"server"`
	CDN        string            `json:"cdn"`
	WAF        string            `json:"waf"`
	Region     string            `json:"region"`
	DurationMS int64             `json:"duration_ms"`
	Error      string            `json:"error,omitempty"`
	ScannedAt  time.Time         `json:"scanned_at"`
}

// scan performs a full forensic inspection of the given URL.
// It validates the URL, blocks SSRF vectors, fetches the resource,
// and returns a populated ScanResult. Errors are non-fatal —
// they are recorded in ScanResult.Error and the result is always returned.
func (app *App) scan(rawURL string) ScanResult {
	result := ScanResult{
		URL:       rawURL,
		Region:    "us-east",
		ScannedAt: time.Now().UTC(),
	}

	// ── 1. Validate + sanitize URL ────────────────────────────────────────
	if err := validateURL(rawURL); err != nil {
		result.Error = err.Error()
		return result
	}

	// ── 2. Fetch ──────────────────────────────────────────────────────────
	start := time.Now()

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("request build failed: %v", err)
		return result
	}

	// Identify ourselves honestly
	req.Header.Set("User-Agent", "404not403-scanner/1.0 (+https://404not403.com)")

	resp, err := app.httpClient.Do(req)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = fmt.Sprintf("fetch failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	// ── 3. Status ─────────────────────────────────────────────────────────
	result.StatusCode = resp.StatusCode

	// ── 4. Headers ────────────────────────────────────────────────────────
	result.Headers = extractHeaders(resp.Header)

	// ── 5. Server ─────────────────────────────────────────────────────────
	result.Server = resp.Header.Get("Server")

	// ── 6. CDN Detection ──────────────────────────────────────────────────
	result.CDN = detectCDN(resp.Header)

	// ── 7. WAF Detection ──────────────────────────────────────────────────
	result.WAF = detectWAF(resp.Header, resp.StatusCode)

	// ── 8. TLS ────────────────────────────────────────────────────────────
	if resp.TLS != nil {
		result.TLSIssuer, result.TLSExpiry = extractTLS(resp.TLS)
	}

	// ── 9. Body hash — read with cap to avoid memory exhaustion ──────────
	bodyHash, bodySize, err := hashBody(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("body read failed: %v", err)
		return result
	}
	result.BodyHash = bodyHash
	result.BodySize = bodySize

	return result
}

// ── URL Validation + SSRF Protection ─────────────────────────────────────────

func validateURL(raw string) error {
	if len(raw) > 2048 {
		return fmt.Errorf("url exceeds maximum length of 2048 characters")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}

	// Scheme must be http or https only
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid scheme: only http and https are permitted")
	}

	// Host must be present
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host")
	}

	// Block SSRF via hostname
	if isBlockedHost(host) {
		return fmt.Errorf("target host is not permitted")
	}

	// Resolve to IP and block SSRF via DNS rebinding
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("host resolution failed: %v", err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("target resolves to a private address")
		}
	}

	return nil
}

// isBlockedHost blocks known dangerous hostnames before DNS resolution.
func isBlockedHost(host string) bool {
	blocked := []string{
		"localhost",
		"metadata.google.internal",
	}
	lower := strings.ToLower(host)
	for _, b := range blocked {
		if lower == b {
			return true
		}
	}
	return false
}

// isPrivateIP returns true if the IP is in a private or reserved range.
// Blocks: loopback, link-local (metadata), RFC1918 private ranges.
func isPrivateIP(ip net.IP) bool {
	private := []string{
		"127.0.0.0/8",    // loopback
		"::1/128",        // IPv6 loopback
		"169.254.0.0/16", // link-local — AWS/GCP/Railway metadata
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"fc00::/7",       // IPv6 unique local
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ── Header Extraction ─────────────────────────────────────────────────────────

// extractHeaders returns a flat map of response headers.
// Multi-value headers are joined with "; ".
// We store all headers for forensic completeness.
func extractHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for key, vals := range h {
		out[strings.ToLower(key)] = strings.Join(vals, "; ")
	}
	return out
}

// ── CDN Detection ─────────────────────────────────────────────────────────────

func detectCDN(h http.Header) string {
	checks := []struct {
		header string
		name   string
	}{
		{"cf-ray", "Cloudflare"},
		{"x-amz-cf-id", "AWS CloudFront"},
		{"x-served-by", "Fastly"},
		{"x-check-cacheable", "Akamai"},
		{"x-vercel-id", "Vercel"},
		{"x-nf-request-id", "Netlify"},
		{"x-railway-request-id", "Railway"},
		{"x-cache", "Generic CDN Cache"},
	}
	for _, c := range checks {
		if h.Get(c.header) != "" {
			return c.name
		}
	}
	return ""
}

// ── WAF Detection ─────────────────────────────────────────────────────────────

func detectWAF(h http.Header, status int) string {
	// Cloudflare WAF — cf-ray present and blocked
	if h.Get("cf-ray") != "" && status == 403 {
		return "Cloudflare WAF"
	}
	// Sucuri
	if h.Get("x-sucuri-id") != "" {
		return "Sucuri WAF"
	}
	// AWS WAF
	if h.Get("x-amzn-requestid") != "" && status == 403 {
		return "AWS WAF"
	}
	// ModSecurity fingerprint in Server header
	server := strings.ToLower(h.Get("Server"))
	if strings.Contains(server, "mod_security") || strings.Contains(server, "modsecurity") {
		return "ModSecurity"
	}
	return ""
}

// ── TLS Inspection ────────────────────────────────────────────────────────────

func extractTLS(state *tls.ConnectionState) (issuer string, expiry time.Time) {
	if len(state.PeerCertificates) == 0 {
		return "", time.Time{}
	}
	cert := state.PeerCertificates[0]
	if cert.Issuer.Organization != nil && len(cert.Issuer.Organization) > 0 {
		issuer = cert.Issuer.Organization[0]
	} else {
		issuer = cert.Issuer.CommonName
	}
	return issuer, cert.NotAfter
}

// ── Body Hashing ──────────────────────────────────────────────────────────────

// hashBody reads up to 10MB of the response body, computes SHA256,
// and returns the hex digest and byte count.
// The 10MB cap prevents memory exhaustion on large responses.
const maxBodyBytes = 10 * 1024 * 1024 // 10MB

func hashBody(body io.Reader) (hexHash string, size int, err error) {
	h := sha256.New()
	limited := io.LimitReader(body, maxBodyBytes)
	n, err := io.Copy(h, limited)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), int(n), nil
}
