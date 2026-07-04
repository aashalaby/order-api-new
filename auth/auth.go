// Package auth wires Zitadel-based authorization for the HTTP API.
//
// Design goals (mirroring the telemetry package):
//   - Fully optional: when ZITADEL_DOMAIN is unset, New returns a
//     pass-through middleware and the API behaves exactly as before —
//     the binary stays runnable with no identity provider.
//   - Pure Go (zitadel-go + zitadel/oidc), so CGO_ENABLED=0 static
//     builds keep working.
//   - Config via env only; no secrets in code. The key file referenced
//     by ZITADEL_KEY_PATH is itself a secret — mount it, never commit it.
//
// Modes (chosen by which env vars are set, checked in order):
//
//	ZITADEL_DOMAIN                    gate: unset => auth disabled
//	ZITADEL_KEY_PATH=/path/key.json   OAuth2 token introspection. Works
//	                                  with Zitadel's default opaque access
//	                                  tokens, at the cost of one call to
//	                                  Zitadel per request (cached briefly).
//	ZITADEL_CLIENT_ID=<id>            Local JWT validation via OIDC
//	                                  discovery + cached JWKS. No
//	                                  per-request round trip, but the
//	                                  client app in Zitadel must be set to
//	                                  issue JWT access tokens ("Auth Token
//	                                  Type: JWT").
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization/oauth"
	zmiddleware "github.com/zitadel/zitadel-go/v3/pkg/http/middleware"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
)

// Middleware guards HTTP handlers. The zero value (and the value returned
// by New when auth is disabled) is a safe pass-through.
type Middleware struct {
	interceptor *zmiddleware.Interceptor[*oauth.IntrospectionContext]
}

// New builds the middleware from the environment. It is a no-op when
// ZITADEL_DOMAIN is unset; it errors when the domain is set but no
// credential mode is configured (misconfiguration should fail fast at
// startup, like a missing DATABASE_URL, not silently run an open API).
func New(ctx context.Context) (*Middleware, error) {
	domain := normalizeDomain(os.Getenv("ZITADEL_DOMAIN"))
	if domain == "" {
		slog.Info("auth disabled: ZITADEL_DOMAIN not set")
		return &Middleware{}, nil
	}

	var (
		initializer authorization.VerifierInitializer[*oauth.IntrospectionContext]
		mode        string
	)
	switch {
	case os.Getenv("ZITADEL_KEY_PATH") != "":
		mode = "introspection"
		initializer = oauth.DefaultAuthorization(os.Getenv("ZITADEL_KEY_PATH"))
	case os.Getenv("ZITADEL_CLIENT_ID") != "":
		mode = "local-jwt"
		initializer = oauth.DefaultJWTAuthorization(os.Getenv("ZITADEL_CLIENT_ID"))
	default:
		return nil, errors.New(
			"ZITADEL_DOMAIN is set but no credentials: set ZITADEL_KEY_PATH " +
				"(introspection) or ZITADEL_CLIENT_ID (local JWT validation)")
	}

	authZ, err := authorization.New(ctx, zitadel.New(domain), initializer)
	if err != nil {
		return nil, err
	}

	slog.Info("auth enabled", "domain", domain, "mode", mode)
	return &Middleware{interceptor: zmiddleware.New(authZ)}, nil
}

// Enabled reports whether requests will actually be checked.
func (m *Middleware) Enabled() bool {
	return m != nil && m.interceptor != nil
}

// Require wraps next so that it only runs for requests carrying a valid
// Bearer token; otherwise the SDK responds 401. Pass-through when auth
// is disabled.
func (m *Middleware) Require(next http.Handler) http.Handler {
	if !m.Enabled() {
		return next
	}
	return m.interceptor.RequireAuthorization()(next)
}

// UserID returns the authenticated subject stored in ctx by the Require
// middleware, or "" when the request was not authenticated (auth disabled,
// a public route, or unit tests). Package-level so handlers can resolve
// ownership without holding a reference to the Middleware — the context is
// the only coupling point.
//
// The recover guard is deliberate: zitadel-go's authorization.UserID
// dereferences the auth context without a nil check, so calling it on a
// request that never went through RequireAuthorization panics with a nil
// pointer SIGSEGV instead of returning "". That is exactly our supported
// no-auth configuration (and every handler unit test), so we absorb the
// panic and report "no authenticated user". The guard costs nothing on
// the authenticated path — recover() is a no-op when nothing panicked.
func UserID(ctx context.Context) (id string) {
	defer func() {
		if recover() != nil {
			id = ""
		}
	}()
	return authorization.UserID(ctx)
}

// UserID is the method form of the package-level UserID helper, kept for
// callers that already hold the Middleware.
func (m *Middleware) UserID(ctx context.Context) string {
	if !m.Enabled() {
		return ""
	}
	return UserID(ctx)
}

// normalizeDomain accepts either a bare domain or a full URL. The zitadel
// SDK expects a bare domain (it prepends https:// itself), but a URL is
// what most people naturally paste — tolerate both rather than failing
// with a confusing "https://https//..." discovery error.
func normalizeDomain(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	return strings.TrimSuffix(v, "/")
}
