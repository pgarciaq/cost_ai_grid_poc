package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Known static paths pass through
		{"/api/v1/events", "/api/v1/events"},
		{"/api/v1/reports/costs", "/api/v1/reports/costs"},
		{"/api/v1/reports/summary", "/api/v1/reports/summary"},
		{"/api/v1/debug/config", "/api/v1/debug/config"},
		{"/healthz", "/healthz"},
		{"/readyz", "/readyz"},
		{"/debug/dashboard", "/debug/dashboard"},
		{"/", "/"},

		// Parameterized paths normalized
		{"/api/v1/quotas/tenant-acme", "/api/v1/quotas/{tenant_id}"},
		{"/api/v1/quotas/tenant-with-dashes-123", "/api/v1/quotas/{tenant_id}"},
		{"/api/v1/customers/cust-001", "/api/v1/customers/{tenant_id}"},
		{"/api/v1/customers/cust-001/entitlements/key/value", "/api/v1/customers/{tenant_id}"},

		// Unknown paths bucketed to prevent cardinality explosion
		{"/random/path", "/other"},
		{"/api/v2/events", "/other"},
		{"/api/v1/unknown", "/other"},
		{"", "/other"},
		{"/x1", "/other"},
		{"/../../etc/passwd", "/other"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := normalizePath(tc.path)
			if got != tc.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestNormalizePath_QuotasExactPrefix(t *testing.T) {
	got := normalizePath("/api/v1/quotas/")
	if got != "/api/v1/quotas/{tenant_id}" {
		t.Errorf("trailing slash should still normalize, got %q", got)
	}
}

func TestStatusWriter_CapturesStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	sw.WriteHeader(http.StatusNotFound)

	if sw.status != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", sw.status)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("underlying recorder: got %d, want 404", rec.Code)
	}
}

func TestStatusWriter_DefaultsTo200(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	if sw.status != http.StatusOK {
		t.Errorf("default status: got %d, want 200", sw.status)
	}
}

func TestHTTPMiddleware_RecordsRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	wrapped := HTTPMiddleware(handler)
	req := httptest.NewRequest("POST", "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("response code: got %d, want 202", rec.Code)
	}
}
