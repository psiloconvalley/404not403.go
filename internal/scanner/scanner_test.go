package scanner

import (
	"net"
	"net/http"
	"testing"
)

// ── URL Validation ───────────────────────────────────────────────────────────

func TestValidateURL_ValidHTTPS(t *testing.T) {
	err := ValidateURL("https://example.com")
	if err != nil {
		t.Errorf("valid HTTPS URL should pass: %v", err)
	}
}

func TestValidateURL_ValidHTTP(t *testing.T) {
	err := ValidateURL("http://example.com")
	if err != nil {
		t.Errorf("valid HTTP URL should pass: %v", err)
	}
}

func TestValidateURL_RejectsEmptyScheme(t *testing.T) {
	err := ValidateURL("example.com")
	if err == nil {
		t.Error("URL without scheme should be rejected")
	}
}

func TestValidateURL_RejectsFTP(t *testing.T) {
	err := ValidateURL("ftp://example.com")
	if err == nil {
		t.Error("FTP scheme should be rejected")
	}
}

func TestValidateURL_RejectsJavascript(t *testing.T) {
	err := ValidateURL("javascript:alert(1)")
	if err == nil {
		t.Error("javascript scheme should be rejected")
	}
}

func TestValidateURL_RejectsNoHost(t *testing.T) {
	err := ValidateURL("https://")
	if err == nil {
		t.Error("URL with no host should be rejected")
	}
}

func TestValidateURL_RejectsTooLong(t *testing.T) {
	long := "https://example.com/" + string(make([]byte, 2048))
	err := ValidateURL(long)
	if err == nil {
		t.Error("URL over 2048 chars should be rejected")
	}
}

// ── SSRF Protection ──────────────────────────────────────────────────────────

func TestIsBlockedHost_Localhost(t *testing.T) {
	if !IsBlockedHost("localhost") {
		t.Error("localhost should be blocked")
	}
}

func TestIsBlockedHost_LocalhostUppercase(t *testing.T) {
	if !IsBlockedHost("LOCALHOST") {
		t.Error("LOCALHOST should be blocked (case insensitive)")
	}
}

func TestIsBlockedHost_MetadataGoogle(t *testing.T) {
	if !IsBlockedHost("metadata.google.internal") {
		t.Error("metadata.google.internal should be blocked")
	}
}

func TestIsBlockedHost_NormalHost(t *testing.T) {
	if IsBlockedHost("google.com") {
		t.Error("google.com should not be blocked")
	}
}

func TestIsPrivateIP_Loopback(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	if !IsPrivateIP(ip) {
		t.Error("127.0.0.1 should be private")
	}
}

func TestIsPrivateIP_Loopback2(t *testing.T) {
	ip := net.ParseIP("127.0.0.99")
	if !IsPrivateIP(ip) {
		t.Error("127.0.0.99 should be private")
	}
}

func TestIsPrivateIP_Metadata(t *testing.T) {
	ip := net.ParseIP("169.254.169.254")
	if !IsPrivateIP(ip) {
		t.Error("169.254.169.254 (metadata) should be private")
	}
}

func TestIsPrivateIP_RFC1918_10(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	if !IsPrivateIP(ip) {
		t.Error("10.0.0.1 should be private")
	}
}

func TestIsPrivateIP_RFC1918_172(t *testing.T) {
	ip := net.ParseIP("172.16.0.1")
	if !IsPrivateIP(ip) {
		t.Error("172.16.0.1 should be private")
	}
}

func TestIsPrivateIP_RFC1918_192(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	if !IsPrivateIP(ip) {
		t.Error("192.168.1.1 should be private")
	}
}

func TestIsPrivateIP_IPv6Loopback(t *testing.T) {
	ip := net.ParseIP("::1")
	if !IsPrivateIP(ip) {
		t.Error("::1 should be private")
	}
}

func TestIsPrivateIP_PublicIP(t *testing.T) {
	ip := net.ParseIP("8.8.8.8")
	if IsPrivateIP(ip) {
		t.Error("8.8.8.8 should not be private")
	}
}

func TestIsPrivateIP_PublicIP2(t *testing.T) {
	ip := net.ParseIP("1.1.1.1")
	if IsPrivateIP(ip) {
		t.Error("1.1.1.1 should not be private")
	}
}

// ── CDN Detection ────────────────────────────────────────────────────────────

func TestDetectCDN_Cloudflare(t *testing.T) {
	h := http.Header{}
	h.Set("cf-ray", "abc123")
	if DetectCDN(h) != "Cloudflare" {
		t.Error("cf-ray header should detect Cloudflare")
	}
}

func TestDetectCDN_Vercel(t *testing.T) {
	h := http.Header{}
	h.Set("x-vercel-id", "abc123")
	if DetectCDN(h) != "Vercel" {
		t.Error("x-vercel-id header should detect Vercel")
	}
}

func TestDetectCDN_Railway(t *testing.T) {
	h := http.Header{}
	h.Set("x-railway-request-id", "abc123")
	if DetectCDN(h) != "Railway" {
		t.Error("x-railway-request-id should detect Railway")
	}
}

func TestDetectCDN_None(t *testing.T) {
	h := http.Header{}
	if DetectCDN(h) != "" {
		t.Error("empty headers should detect no CDN")
	}
}

// ── WAF Detection ────────────────────────────────────────────────────────────

func TestDetectWAF_Cloudflare403(t *testing.T) {
	h := http.Header{}
	h.Set("cf-ray", "abc123")
	if DetectWAF(h, 403) != "Cloudflare WAF" {
		t.Error("cf-ray + 403 should detect Cloudflare WAF")
	}
}

func TestDetectWAF_Cloudflare200(t *testing.T) {
	h := http.Header{}
	h.Set("cf-ray", "abc123")
	if DetectWAF(h, 200) != "" {
		t.Error("cf-ray + 200 should not detect WAF")
	}
}

func TestDetectWAF_Sucuri(t *testing.T) {
	h := http.Header{}
	h.Set("x-sucuri-id", "abc123")
	if DetectWAF(h, 200) != "Sucuri WAF" {
		t.Error("x-sucuri-id should detect Sucuri WAF")
	}
}

func TestDetectWAF_None(t *testing.T) {
	h := http.Header{}
	if DetectWAF(h, 200) != "" {
		t.Error("empty headers should detect no WAF")
	}
}
