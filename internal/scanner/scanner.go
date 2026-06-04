package scanner

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

	"github.com/psiloconvalley/404not403/internal/app"
)

// ScanResult is the single source of truth for one URL inspection.
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

// Scan performs a full forensic inspection of the given URL.
func Scan(a *app.App, rawURL string) ScanResult {
	result := ScanResult{
		URL:       rawURL,
		Region:    "us-east",
		ScannedAt: time.Now().UTC(),
	}

	if err := ValidateURL(rawURL); err != nil {
		result.Error = err.Error()
		return result
	}

	start := time.Now()
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("request build failed: %v", err)
		return result
	}

	req.Header.Set("User-Agent", "404not403-scanner/1.0 (+https://404not403.com)")

	resp, err := a.HTTPClient.Do(req)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = fmt.Sprintf("fetch failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Headers = ExtractHeaders(resp.Header)
	result.Server = resp.Header.Get("Server")
	result.CDN = DetectCDN(resp.Header)
	result.WAF = DetectWAF(resp.Header, resp.StatusCode)

	if resp.TLS != nil {
		result.TLSIssuer, result.TLSExpiry = ExtractTLS(resp.TLS)
	}

	bodyHash, bodySize, err := HashBody(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("body read failed: %v", err)
		return result
	}
	result.BodyHash = bodyHash
	result.BodySize = bodySize

	return result
}

func ValidateURL(raw string) error {
	if len(raw) > 2048 {
		return fmt.Errorf("url exceeds maximum length of 2048 characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid scheme: only http and https are permitted")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host")
	}
	if IsBlockedHost(host) {
		return fmt.Errorf("target host is not permitted")
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("host resolution failed: %v", err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if IsPrivateIP(ip) {
			return fmt.Errorf("target resolves to a private address")
		}
	}
	return nil
}

func IsBlockedHost(host string) bool {
	blocked := []string{"localhost", "metadata.google.internal"}
	lower := strings.ToLower(host)
	for _, b := range blocked {
		if lower == b {
			return true
		}
	}
	return false
}

func IsPrivateIP(ip net.IP) bool {
	private := []string{
		"127.0.0.0/8", "::1/128", "169.254.0.0/16",
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
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

func ExtractHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for key, vals := range h {
		out[strings.ToLower(key)] = strings.Join(vals, "; ")
	}
	return out
}

func DetectCDN(h http.Header) string {
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

func DetectWAF(h http.Header, status int) string {
	if h.Get("cf-ray") != "" && status == 403 {
		return "Cloudflare WAF"
	}
	if h.Get("x-sucuri-id") != "" {
		return "Sucuri WAF"
	}
	if h.Get("x-amzn-requestid") != "" && status == 403 {
		return "AWS WAF"
	}
	server := strings.ToLower(h.Get("Server"))
	if strings.Contains(server, "mod_security") || strings.Contains(server, "modsecurity") {
		return "ModSecurity"
	}
	return ""
}

func ExtractTLS(state *tls.ConnectionState) (string, time.Time) {
	if len(state.PeerCertificates) == 0 {
		return "", time.Time{}
	}
	cert := state.PeerCertificates[0]
	issuer := ""
	if cert.Issuer.Organization != nil && len(cert.Issuer.Organization) > 0 {
		issuer = cert.Issuer.Organization[0]
	} else {
		issuer = cert.Issuer.CommonName
	}
	return issuer, cert.NotAfter
}

func HashBody(body io.Reader) (string, int, error) {
	const maxBodyBytes = 10 * 1024 * 1024
	h := sha256.New()
	limited := io.LimitReader(body, maxBodyBytes)
	n, err := io.Copy(h, limited)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), int(n), nil
}
