# order-api web

The product UI for order-api: Next.js (App Router) + Auth.js v5 with the
Zitadel provider, acting as a **BFF** in front of the Go service.

Why this shape (decided in HANDOFF): token custody. The OIDC Authorization
Code + PKCE flow terminates server-side in this app; tokens live in an
encrypted, HttpOnly session cookie and are never exposed to browser
storage or client JS. Route handlers under `/api/orders*` proxy to the Go
API and attach the Bearer token, so the browser only ever talks
same-origin — **no CORS anywhere in the stack, by design.**

The vanilla console embedded in the Go binary at `/` stays what it is: the
API test bench. This app is the product.

## One-time setup

### 1. Generate and commit the lockfile

```bash
cd web
npm install         # creates package-lock.json
git add package-lock.json
```

CI (`web` job) and the Dockerfile both use `npm ci`, which requires the
lockfile. The CI job fails with a pointed message if it's missing.

### 2. Zitadel configuration (login AND registration — no custom forms)

All in the Zitadel console of the self-hosted instance:

**Instance — enable self-registration**
- Settings → Login Behavior and Security: enable **Register allowed**.
  Optionally require verified email addresses (Notification settings must
  have SMTP configured for verification mails).
- Branding: customize the hosted login/register pages if desired.

**Project — one project holds both apps**
- Note the **project ID** (project detail page) — the web app requests the
  `urn:zitadel:iam:org:project:id:<PROJECT_ID>:aud` scope so access tokens
  carry the project in `aud`, which is what makes the Go API accept them.

**Application 1 — this web app (new)**
- Type: **Web** → **Code** (confidential, Authorization Code + PKCE).
- Redirect URIs:
  - `https://<web-domain>/api/auth/callback/zitadel`
  - `http://localhost:3000/api/auth/callback/zitadel` (enable *dev mode*
    on the app so the plain-http localhost redirect is allowed)
- Post-logout URIs: `https://<web-domain>/` and `http://localhost:3000/`
- **Token settings → Auth Token Type: JWT.** Required — the Go API
  validates access tokens locally (`ZITADEL_CLIENT_ID` mode) and can only
  do that with JWT access tokens, not Zitadel's default opaque ones.
- Copy the client ID and the client secret (shown once).

**Application 2 — the Go API (keep/confirm existing)**
- The API application (or JWT-mode client) in the same project. Its client
  ID is the Go container's `ZITADEL_CLIENT_ID`.

### 3. Environment

Copy `.env.example` to `.env` for local dev; on the deployed Scaleway
container, set the same variables on the container resource (CI never
touches runtime env — it only swaps the image):

| Variable | What |
|---|---|
| `AUTH_SECRET` | Session-cookie encryption key (`npx auth secret`) |
| `AUTH_ZITADEL_ISSUER` | `https://<zitadel-domain>` |
| `AUTH_ZITADEL_ID` / `AUTH_ZITADEL_SECRET` | The **web** application's credentials |
| `ZITADEL_PROJECT_ID` | Project ID for the audience scope |
| `ORDER_API_URL` | Internal URL of the Go container |
| `AUTH_URL` | Public base URL of this app |
| `AUTH_TRUST_HOST` | `true` (running behind Scaleway's ingress) |

## Local development

```bash
cd web
npm install
cp .env.example .env   # fill in values
npm run dev            # http://localhost:3000
```

Run the Go API alongside (`DATABASE_URL=... go run .` from the repo root);
point `ORDER_API_URL` at it. With the Zitadel env unset on the Go side,
the API runs authless and the BFF's Bearer header is simply ignored —
handy for pure UI work, though everything lands under the shared
`anonymous` owner.

## Quality gate

```bash
npm run lint        # eslint (next lint was removed in Next 16)
npm run typecheck   # tsc --noEmit
npm run build       # next build (standalone output)
```

The `web` CI job runs exactly these three.

## Deployment

`web/Dockerfile` builds the standalone output into a non-root
`node:22-alpine` runtime listening on **port 3000** (set the Scaleway
container port accordingly). CI builds/pushes
`.../order-api-web:{sha,latest}` and PATCHes the container identified by
the `SCW_WEB_CONTAINER_ID` repo variable — the exact same deploy pattern
as the Go service, matrix-expanded in `ci.yaml`.

## Known simplifications (deliberate)

- **No refresh-token rotation.** The session maxAge is aligned with
  Zitadel's default 12h access-token TTL; on expiry the BFF returns 401
  and the UI routes the user back through hosted login (instant when the
  Zitadel SSO session is still alive). If longer sessions become a real
  need, implement the Auth.js refresh-rotation pattern in the `jwt`
  callback with the `offline_access` scope (must also be enabled on the
  Zitadel app).
- **Sign-out is local.** It clears this app's session cookie but not the
  Zitadel SSO session (no federated logout via `end_session` yet), so
  "Sign in" right after may not prompt for credentials.
