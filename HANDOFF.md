# Handoff: order-api — Web Frontend & User Login/Registration

## What this project is
A REST API in Go for managing orders — graduated from pure POC: it now has
observability, authentication, a test console, and continuous deployment to
Scaleway Serverless Containers. Repo: github.com/<owner>/order-api (public).

## Current architecture (all working, CI green)
- **Go 1.26**, stdlib `net/http`, method+path ServeMux routing
- **Endpoints:** GET/POST /orders, GET/PUT/DELETE /orders/{id} — all five
  individually wrapped in auth middleware; `GET /` serves the embedded console
- **DB layer:** sqlc-generated (`db/`, `emit_interface: true` → `db.Querier`)
  over pgx/v5 `pgxpool`, Postgres on Neon; config via `DATABASE_URL` env only
- **Handlers:** `handlers/order_handler.go`; `Server{Queries db.Querier}` DI;
  unit tests with `stubQueries` fake; benchmarks in
  `handlers/order_handler_bench_test.go` (handler layer via stub + price
  conversion sub-benchmarks)
- **Telemetry (`telemetry/`):** OTLP/HTTP traces + metrics; **no-op when
  `OTEL_EXPORTER_OTLP_ENDPOINT` unset**. otelhttp wraps the mux (http.route
  derived automatically from mux patterns — do NOT reintroduce the removed
  otelhttp.WithRouteTag); otelpgx tracer on pool config + RecordStats pool
  metrics. Exporting to New Relic:
  `OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.nr-data.net`,
  `OTEL_EXPORTER_OTLP_HEADERS="api-key=<license>"`,
  `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta` (NR is a delta
  system; cumulative works but wastes ingest)
- **Auth (`auth/`):** Zitadel via zitadel-go v3; **no-op when
  `ZITADEL_DOMAIN` unset**, fail-fast if half-configured. Modes:
  `ZITADEL_KEY_PATH` (introspection, works with opaque tokens, one round trip
  per request) or `ZITADEL_CLIENT_ID` (local JWT validation via JWKS — client
  app in Zitadel must issue JWT access tokens). Domain env accepts bare host
  or URL (normalized). `auth.UserID(ctx)` helper exposes the subject —
  currently unused, intended for per-user data (see goals).
  Zitadel instance: self-hosted on Scaleway serverless functions (nl-ams);
  cold starts can delay discovery/introspection — retry before debugging.
- **Test console:** `frontend/index.html`, single-file vanilla JS, embedded
  via go:embed, served same-origin at `/` (public; data routes stay guarded).
  Wire log shows method/path/status/latency per request; bearer token field
  (sessionStorage). NOTE: API responses use Go field names (`ID`, `Item`,
  `Quantity`, `Price` — db.Order has no JSON tags) while request bodies use
  lowercase keys. The console handles this asymmetry; see goals re: fixing it.
- **Server:** http.Server with timeouts + graceful shutdown (drains, then
  flushes OTel batcher). slog JSON logging. `version` var stamped via ldflags.
- **Docker:** multi-stage, cross-compiled static binary, chainguard/static,
  non-root. go:embed bakes the console in — no Dockerfile changes needed.
- **CI (`.github/workflows/ci.yaml`):** quality (tidy, sqlc drift, vet,
  golangci-lint v2, race tests + coverage, govulncheck) → docker (buildx
  multi-arch; push on main to Scaleway registry
  `rg.nl-ams.scw.cloud/$SCW_REGISTRY_NAMESPACE/order-api:{sha,latest}`) →
  deploy (main only, `production` environment: PATCH container to SHA tag via
  Scaleway HTTP API, poll until ready/error). Deliberately no third-party
  deploy actions touching credentials.
  GitHub config: secret `SCW_SECRET_KEY`; variables `SCW_REGISTRY_NAMESPACE`,
  `SCW_CONTAINER_ID`. Container runtime env (DATABASE_URL, OTEL_*, ZITADEL_*)
  lives on the container resource, set once — CI only swaps the image.
  Deployed container should use ZITADEL_CLIENT_ID (JWT) mode: serverless has
  env vars, not mounted files, so the key.json mode doesn't translate.
- **Benchmarks workflow (`benchmarks.yml`):** workflow_dispatch, runs suite on
  branch + main, benchstat delta in job summary. Not a blocking gate (noisy
  shared runners). Baseline (arm64 dev container): GetOrders ~3.3µs/1 order
  scaling linearly ~280ns/order; writes ~4.6µs; price conversion ~250ns
  (strconv variant ~17% faster — measured, judged not worth switching).
- **DB migrations:** `migrations/*.sql`, Bytebase pipeline (release.yml
  dev→test→prod, OIDC; sql-review.yml on PRs). Append-only, always.

## Known simplifications (deliberate, now candidates — see goals)
- No pagination on GET /orders (benchmarked: linear, fine at POC scale)
- DELETE returns 200 for missing IDs (`:execrows` + 404 still recommended)
- PUT zeroes omitted fields; price as float64 (precision caveat)
- Client supplies order ID on POST — awkward for a real frontend
- db.Order has no JSON tags → PascalCase responses vs lowercase requests
- Orders have no owner — anyone with a valid token sees/mutates everything

## NEXT SESSION GOALS

### 1. Professional web frontend (decision made: Next.js, BFF pattern)
Rationale: with Zitadel in play, the deciding factor is token custody.
Next.js App Router + Auth.js (NextAuth v5, Zitadel provider) terminates the
OIDC Authorization Code + PKCE flow server-side; tokens live in an encrypted
session cookie, never in browser storage. Next route handlers proxy
`/api/orders*` to the Go service, attaching the Bearer token — same-origin
from the browser's perspective, so STILL no CORS anywhere in the stack.
- New `web/` directory (Next.js + TypeScript + Tailwind), monorepo style
- `next.config`: `output: "standalone"` → containerize → deploy as a SECOND
  Scaleway serverless container reusing the exact ci.yaml deploy pattern
  (new job or matrix; new SCW_CONTAINER_ID variable for the web container)
- Env: `AUTH_SECRET`, `AUTH_ZITADEL_ISSUER=https://<zitadel-domain>`,
  `AUTH_ZITADEL_ID`, `AUTH_ZITADEL_SECRET` (confidential web app), and
  `ORDER_API_URL` (internal URL of the Go container)
- Keep the embedded vanilla console at Go `/` — it's the API test bench;
  the Next app is the product UI. Consider moving console to `/console`
  eventually if `/` should redirect to the web app.
- CI: add a `web` job (npm ci, lint, typecheck, build) parallel to `quality`

### 2. Login & registration (decision made: Zitadel-hosted pages)
Do NOT build custom registration forms. Configure Zitadel:
- Enable self-registration on the instance (Settings → Login behavior);
  optionally require email verification; branding via Zitadel's customizer
- Create a **Web application** (confidential, code + PKCE) in the project for
  the Next.js app: redirect URI `https://<web>/api/auth/callback/zitadel`
  (+ localhost equivalent), post-logout URI
- Keep/confirm the **API application** (or JWT-mode client) for the Go
  service; access token type JWT so the Go API validates locally
- Auth.js wiring: Zitadel provider with issuer above; `signIn()` sends users
  to Zitadel's hosted login, which includes the register link — login AND
  registration solved by configuration
- Frontend UX: signed-out landing page → sign in / register buttons;
  signed-in: orders CRUD UI calling the BFF proxy routes

### 3. Backend work to make users meaningful
- **Per-user orders:** append-only migration adding `user_id TEXT NOT NULL`
  (+ index) through Bytebase; sqlc queries become owner-scoped
  (WHERE user_id = $1); handlers pass `auth.UserID(r.Context())`. Decide
  backfill story for existing POC rows (throwaway data — a default owner or
  truncate in dev is fine, but via migration, not manual SQL).
- **API contract cleanup while the only consumer is being rebuilt** (breaking
  changes are cheap right now, and the new frontend removes the excuse):
  server-generated order IDs on POST; `emit_json_tags: true` in sqlc.yaml
  (regenerate, update console + handler tests) for consistent lowercase JSON;
  optionally fix DELETE 404 via `:execrows`.
- `GET /healthz` (public, no auth) — useful for Scaleway health checks.

### Constraints & conventions to preserve
- Quality gate stays green: tidy, sqlc drift, vet, golangci-lint (errcheck
  enforced — handle every returned error), race tests, govulncheck
- No secrets in code or committed files; config via env vars; key.json and
  .env files must never enter the repo (the old DB credential was rotated
  after being committed once — don't repeat it; note the Zitadel key.json
  currently sits in the working tree locally — keep it git-ignored or move it)
- Optional-by-default modularity: telemetry, auth (and anything new) must
  no-op cleanly when unconfigured; fail fast when half-configured
- Static CGO_ENABLED=0 binary; pure-Go deps only
- Update handler tests when Server/middleware wiring changes; benchmarks live
  beside them and must keep compiling
- Migrations append-only via Bytebase; never edit history
- No CORS by design (same-origin console; BFF proxy for the web app) — if
  something seems to need CORS, the architecture took a wrong turn
- Action/CLI versions in workflows were current as of Jul 2026; verify
  marketplace majors before bumping

---

# Execution report — 2026-07-04

All three goals are implemented in this tree. Nothing below was compiled or
`npm install`ed in the environment where the work was done (no Go toolchain,
no network), so **CI is the verifier** — plus the manual steps listed at the
end, which only you can do (credentials, Zitadel console, Scaleway console).

## Goal 3 — Backend: per-user orders, server IDs, DELETE 404, /healthz

- `migrations/202607031200_add_user_id_to_orders.up.sql` — append-only:
  `ADD COLUMN user_id TEXT NOT NULL DEFAULT 'legacy:pre-auth'`, then
  `DROP DEFAULT`, plus `idx_orders_user_id`. Existing rows become
  `legacy:pre-auth` (visible to nobody once auth is on — intentional).
  Apply via Bytebase as usual.
- `db/queries.sql` — every query owner-scoped (`WHERE user_id = $…`);
  `DeleteOrder` is now `:execrows`; `CreateOrder` takes `user_id`.
- `sqlc.yaml` — `emit_json_tags: true` (lowercase JSON keys on `db.Order`).
- `db/*.go` — hand-written to match sqlc 1.31.1 output because sqlc could
  not run here. **Run `sqlc generate` locally**; the CI drift check
  (`git diff --exit-code db/`) will catch any byte-level mismatch.
- `auth/auth.go` — package-level `auth.UserID(ctx)` added.
- `handlers/order_handler.go` — owner from `auth.UserID`, falling back to
  `"anonymous"` when auth is off (preserves no-auth dev mode); POST ignores
  any client-sent `id` and generates `ord_` + 16 base32 chars from
  crypto/rand; DELETE returns 404 on zero rows; GET /orders returns `[]`
  not `null`; new `Healthz` handler.
- `main.go` — `GET /healthz` registered outside the auth wrapper (public)
  and excluded from otelhttp spans (`WithFilter`), so uptime probes don't
  pollute traces.
- Tests and benchmarks updated for the new `db.Querier` signatures; new
  tests cover server-generated IDs, DELETE 404, empty-list JSON, healthz.
- `frontend/index.html` (test console) — lowercase JSON keys, ID field
  removed from the create form.

## Goal 1 — Next.js BFF (`web/`)

- Next 16.2 + Auth.js v5 (next-auth beta.31) + Zitadel provider. Access
  token lives only in the encrypted session cookie (jwt callback); there is
  deliberately **no session callback** — the token is never exposed to
  client JS.
- `web/src/lib/order-api.ts` — server-side proxy: reads the token with
  `getToken`, forwards `Authorization: Bearer` to `ORDER_API_URL`, passes
  status/body through. Route handlers: `/api/orders` (GET/POST) and
  `/api/orders/[id]` (GET/PUT/DELETE).
- UI: signed-out landing with Sign in / Create account (both go to
  Zitadel-hosted pages; registration uses `prompt=create`), signed-in
  orders ledger (full CRUD, receipt-style design, no browser storage).
- `web/Dockerfile` — 3-stage node:22-alpine, standalone output, non-root,
  port 3000, no secrets at build time.
- Simplifications (documented in `web/README.md`): no refresh-token
  rotation — on expiry the BFF returns 401 and the UI prompts re-login;
  sign-out is local (no Zitadel end-session redirect).

## Goal 2 — Zitadel-hosted login/registration

Config-only, as required. The step-by-step is in `web/README.md`:
enable self-registration (Login Behavior), create a **Web** application
(Code + PKCE, confidential) with the `/api/auth/callback/zitadel` redirect
URIs, set **Auth Token Type: JWT** (required for the Go API's local JWT
validation mode), and grant the project audience scope so the same access
token is accepted by the Go API.

## CI (`.github/workflows/ci.yaml`)

- New `web` job: guard for a committed `web/package-lock.json` (clear error
  if missing), then `npm ci` → lint → typecheck → production build.
- `docker` job is now a matrix: `api` (multi-arch, as before) and `web`
  (amd64 only — npm under QEMU is very slow and Scaleway runs amd64).
  Separate GHA cache scopes per leg.
- `deploy` job is a matrix over two containers:
  `vars.SCW_CONTAINER_ID` (api) and `vars.SCW_WEB_CONTAINER_ID` (web),
  with a clear error if the variable isn't set.

## Manual steps required (in order)

1. **SECURITY — rotate exposed credentials.** `Env-Setup.txt` in the
   public repo's history contains a live GitHub PAT and the Bytebase
   admin password. The working-copy file has been redacted, but history
   still has them: **revoke the PAT in GitHub → Settings → Developer
   settings**, **change the Bytebase password**, then
   `git rm Env-Setup.txt` (or keep the redacted version) and consider
   history rewriting or treating the repo history as burned.
2. `go mod tidy` — go.mod/go.sum in the tree are stale (missing
   zitadel-go, otelpgx, otelhttp entries added in earlier work).
3. `sqlc generate` — confirm the hand-written `db/` matches; commit any
   diff sqlc produces.
4. `cd web && npm install` — commit `web/package-lock.json` (the CI web
   job hard-fails without it, by design).
5. `go test ./...` locally, then push and let the quality gates run.
6. Apply the migration through Bytebase.
7. Zitadel console: follow `web/README.md` (self-registration, Web app,
   JWT token type, project audience).
8. Scaleway: create the second Serverless Container for the web image
   (port 3000, runtime env per `web/.env.example`), add its UUID as repo
   variable `SCW_WEB_CONTAINER_ID`.

---

# Update — 2026-07-04 (later): boozoo.top, Terraform, New Relic

## What changed

- **`infra/`** — new Terraform stack (Scaleway + Cloudflare providers,
  state in Scaleway Object Storage). Owns the container namespace, both
  Serverless Containers (sizing, env, secrets, image tag), the custom
  domains `orders.boozoo.top` (web) and `api.boozoo.top` (api), and the
  Cloudflare CNAME records. See `infra/README.md` for the full rationale,
  bootstrap steps, and day-2 notes.
- **CI** — the curl-PATCH deploy job is replaced by `terraform apply`
  with `image_tag = <commit sha>`; a new `infra-check` job runs
  `fmt`/`validate` on PRs. Repo variables `SCW_CONTAINER_ID` /
  `SCW_WEB_CONTAINER_ID` are obsolete.
- **Telemetry** — new `telemetry/rtt.go` + wiring in `main.go`: a 30s
  `db.client.rtt` histogram capturing the pure network/protocol floor to
  Neon, so query-span latency can be decomposed into network vs.
  execution instead of blaming the database wholesale. Activity-gated:
  probes only while real `/orders*` traffic is flowing, so it never
  defeats Neon's autosuspend. New Relic export
  is pure config: the stack sets the OTLP endpoint + `api-key` header
  from `NEW_RELIC_LICENSE_KEY` (empty key = telemetry off, unchanged
  default).

## New manual steps (once)

1. Create the private state bucket: `scw object bucket create
   order-api-tfstate region=nl-ams`.
2. Cloudflare: API token scoped to Zone.DNS:Edit on boozoo.top; copy the
   zone ID.
3. Add GitHub secrets `SCW_ACCESS_KEY`, `SCW_PROJECT_ID`,
   `CLOUDFLARE_API_TOKEN`, `DATABASE_URL`, `AUTH_SECRET`,
   `AUTH_ZITADEL_SECRET`, `NEW_RELIC_LICENSE_KEY` and variables
   `CLOUDFLARE_ZONE_ID`, `ZITADEL_CLIENT_ID_API`, `AUTH_ZITADEL_ID`,
   `ZITADEL_PROJECT_ID` (values from the Zitadel setup already done).
4. Merge to main → first apply creates everything; certificates appear a
   few minutes after DNS propagates.
5. Zitadel: add `https://orders.boozoo.top/api/auth/callback/zitadel`
   redirect + `https://orders.boozoo.top/` post-logout on `order-web`.
6. Delete the hand-created containers from the old namespace, and the
   now-unused `SCW_CONTAINER_ID`/`SCW_WEB_CONTAINER_ID` repo variables.


---

# Update — 2026-07-04 (later still): Zitadel fully Terraform-managed

`infra/zitadel.tf` now owns the complete Zitadel configuration: the
`order-api` project, the `order-api-backend` API application, the
`order-web` OIDC application (code+PKCE, JWT access tokens, dev-mode
localhost redirects), the login policy (self-registration on, phone
login off, OTP/U2F second factors), and the branding label policy (app
palette, hidden loginname suffix, **watermark removed**). Client IDs,
the web client secret, and the project ID flow directly from these
resources into the container env — the `ZITADEL_CLIENT_ID_API`,
`AUTH_ZITADEL_ID`, `ZITADEL_PROJECT_ID` variables and
`AUTH_ZITADEL_SECRET` secret are gone (the bootstrap script deletes them
if present). New inputs: `ZITADEL_ORG_ID` variable + `ZITADEL_PAT`
secret (machine user `terraform`, Org Owner — see infra/README.md
bootstrap steps, which also cover deleting or importing the
console-created project).

---

# Session handoff — 2026-07-05

## Current state

Everything through identity, infra, and observability is code-managed and
CI-deployed:

- **Backend** (Go 1.26): per-user orders via `auth.UserID(ctx)`
  (panic-safe wrapper; "anonymous" fallback when auth is off),
  server-generated `ord_…` IDs, DELETE→404 via `:execrows`, lowercase
  JSON, public `/healthz`, OTel traces + pgxpool metrics + activity-gated
  `db.client.rtt` probe (never keeps Neon awake when idle).
- **Web** (`web/`, Next 16.2 + Auth.js beta.31): BFF pattern, token only
  in encrypted cookie, `/api/orders*` proxy, Zitadel-hosted
  login/registration, lint clean (one documented upstream-bug
  suppression: facebook/react#34905).
- **Infra** (`infra/`): Terraform owns Scaleway containers
  (`orders.boozoo.top` / `api.boozoo.top`), Cloudflare DNS-only CNAMEs,
  and ALL of Zitadel (project, API+Web apps, login policy with
  self-registration, branding with watermark off). Client IDs/secret flow
  resource→container env. State: `order-api-tfstate` bucket (nl-ams).
- **CI**: quality (Go) + web (npm) + infra-check (PR) → docker matrix →
  terraform apply with `image_tag = SHA`. Actions: checkout v7,
  docker/login v4, setup-terraform v4.
- **Bootstrap**: `scripts/bootstrap-github.sh` sets all 8 secrets / 3
  variables; obsolete entries auto-deleted.

Verified working by the user: local dev end-to-end (Zitadel login,
manual email verification, CRUD), lint/typecheck/build, go test.
First full production apply pending at handoff time.

## Next session: payments via polar.sh

Intended work: add payment support through polar.sh. Relevant
integration points in the current architecture:

- **Where checkout starts**: `web/src/components/OrdersLedger.tsx`
  (client) → BFF route handlers under `web/src/app/api/` — a checkout
  session creation belongs in a new server-side route (Polar access token
  must stay server-side, same custody rule as the Zitadel token).
- **Webhooks**: Polar → a new public Go endpoint (register outside the
  auth wrapper in `main.go`, like `/healthz`; verify Polar's webhook
  signature) or a Next route handler. Order state transitions
  (paid/refunded) mean new columns → **append-only migration** via
  Bytebase + `sqlc generate` (CI enforces drift).
- **Secrets**: Polar access token + webhook secret should enter through
  `infra/variables.tf` → container `secret_environment_variables`, and
  `scripts/bootstrap.env.example` + `bootstrap-github.sh` extended —
  same pattern as `ZITADEL_PAT`.
- **User identity ↔ Polar customer**: orders carry `user_id` (Zitadel
  sub); Polar has an external-customer-ID concept that should map to it.

Constraints that still apply: no CORS by design (everything same-origin
through the BFF), optional-by-default modularity (payments should no-op
without config, like auth/telemetry), no secrets in the repo, CI is the
verifier.
