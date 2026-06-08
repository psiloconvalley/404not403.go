package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/psiloconvalley/404not403/internal/app"
)

// ── Health Tests ────────────────────────────────────────────────────────────

func TestHealth_NoDB_ReturnsOffline(t *testing.T) {
	a := &app.App{DB: nil}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	Health(a).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"offline"`) {
		t.Errorf("expected offline status, got: %s", body)
	}
}

func TestHealth_NoDB_ReturnsJSON(t *testing.T) {
	a := &app.App{DB: nil}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	Health(a).ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got: %s", ct)
	}
}

// ── Home Tests ──────────────────────────────────────────────────────────────

func testTemplates() *template.Template {
	tmpl := template.New("test")
	template.Must(tmpl.New("index.html").Parse("<html>home</html>"))
	template.Must(tmpl.New("404.html").Parse("<html>not found</html>"))
	template.Must(tmpl.New("status.html").Parse("<html>status {{.DBStatus}}</html>"))
	template.Must(tmpl.New("billing-success.html").Parse("<html>success</html>"))
	template.Must(tmpl.New("billing-cancel.html").Parse("<html>cancel</html>"))
	return tmpl
}

func TestHome_RootPath_Returns200(t *testing.T) {
	a := &app.App{Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	Home(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHome_NonRootPath_Returns404(t *testing.T) {
	a := &app.App{Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	Home(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHome_NonRootPath_Renders404Template(t *testing.T) {
	a := &app.App{Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/does-not-exist", nil)
	w := httptest.NewRecorder()

	Home(a).ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "not found") {
		t.Errorf("expected 404 template content, got: %s", body)
	}
}

// ── Status Tests ────────────────────────────────────────────────────────────

func TestStatus_NoDB_RendersDegraded(t *testing.T) {
	a := &app.App{DB: nil, Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()

	Status(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "offline") {
		t.Errorf("expected 'offline' in status page, got: %s", body)
	}
}

// ── Billing Page Tests ──────────────────────────────────────────────────────

func TestBillingSuccess_Returns200(t *testing.T) {
	a := &app.App{Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/billing/success", nil)
	w := httptest.NewRecorder()

	BillingSuccess(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "success") {
		t.Errorf("expected success content, got: %s", body)
	}
}

func TestBillingCancel_Returns200(t *testing.T) {
	a := &app.App{Templates: testTemplates()}
	req := httptest.NewRequest("GET", "/billing/cancel", nil)
	w := httptest.NewRecorder()

	BillingCancel(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cancel") {
		t.Errorf("expected cancel content, got: %s", body)
	}
}
