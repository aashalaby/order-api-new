package auth

// These tests cover the paths that don't require a live Zitadel instance:
// the disabled pass-through and the fail-fast misconfiguration error.
// The enabled paths (introspection / JWT validation) perform OIDC
// discovery against the real domain at startup, so they're exercised by
// integration testing, not unit tests.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisabledIsPassThrough(t *testing.T) {
	t.Setenv("ZITADEL_DOMAIN", "")

	m, err := New(context.Background())
	if err != nil {
		t.Fatalf("New with unset domain: unexpected error %v", err)
	}
	if m.Enabled() {
		t.Fatal("Enabled() = true, want false when ZITADEL_DOMAIN is unset")
	}

	called := false
	h := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	// Deliberately no Authorization header: disabled auth must not care.
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders", nil))

	if !called {
		t.Fatal("next handler was not called through disabled middleware")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestZeroValueIsSafe(t *testing.T) {
	var m Middleware
	if m.Enabled() {
		t.Fatal("zero value Enabled() = true, want false")
	}
	if got := m.UserID(context.Background()); got != "" {
		t.Fatalf("zero value UserID() = %q, want empty", got)
	}
}

func TestDomainWithoutCredentialsFailsFast(t *testing.T) {
	t.Setenv("ZITADEL_DOMAIN", "example.zitadel.cloud")
	t.Setenv("ZITADEL_KEY_PATH", "")
	t.Setenv("ZITADEL_CLIENT_ID", "")

	if _, err := New(context.Background()); err == nil {
		t.Fatal("New with domain but no credentials: want error, got nil")
	}
}

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"example.zitadel.cloud":           "example.zitadel.cloud",
		"https://example.zitadel.cloud":   "example.zitadel.cloud",
		"https://example.zitadel.cloud/":  "example.zitadel.cloud",
		"http://localhost:8081":           "localhost:8081",
		"  https://x.functions.scw.cloud": "x.functions.scw.cloud",
		"": "",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// Regression: zitadel-go's authorization.UserID panics (nil pointer) when
// the context carries no auth context — the situation on every request in
// no-auth mode, on public routes, and in handler unit tests. Our wrapper
// must absorb that and report "no user" instead of taking the process down.
func TestUserIDWithoutAuthContextIsEmpty(t *testing.T) {
	if got := UserID(context.Background()); got != "" {
		t.Fatalf("UserID(bare ctx) = %q, want empty", got)
	}
}
