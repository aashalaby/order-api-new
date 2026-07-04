# infra

Terraform stack for order-api production: two Scaleway Serverless
Containers (Go API + Next.js web), their custom domains on **boozoo.top**,
the Cloudflare CNAME records, and the **entire Zitadel configuration** —
project, both applications, login policy (self-registration), and
branding (theme, watermark off). Applied automatically by the `deploy`
job in CI on every merge to main, with the freshly built image SHA.

Because Zitadel is in the stack, the client IDs, the web client secret,
and the project ID flow directly from the Zitadel resources into the
container environments — no copy-paste, no GitHub variables for them.

```
CI docker job          pushes  order-api:<sha>, order-api-web:<sha>
CI deploy job          terraform apply -var image_tag=<sha>
        └── containers.tf   two containers, env + secrets, rollout on tag change
        └── dns.tf          Cloudflare CNAMEs → Scaleway domains → LE certs
```

## Why Terraform (and not something else)

- The task is *declarative infrastructure spanning two providers*
  (Scaleway + Cloudflare) that both have first-class Terraform providers —
  the textbook Terraform case. Ordering (CNAME before domain validation),
  drift detection, and teardown come free.
- Alternatives considered: Pulumi does the same with real code but adds a
  language runtime to CI for no gain at this size; raw `scw`/`curl`
  scripting (the previous deploy job) can't create-or-update idempotently
  or manage Cloudflare; Scaleway's official Terraform module for
  serverless containers exists but assumes Scaleway-hosted DNS, and our
  zone is on Cloudflare — so plain resources it is.

## One-time bootstrap (in order)

1. **State bucket** (private — the state will contain secrets):
   `scw object bucket create order-api-tfstate region=nl-ams`
2. **Cloudflare API token**: dashboard → My Profile → API Tokens →
   Create → template "Edit zone DNS" → scope it to the single zone
   boozoo.top. Also copy the **Zone ID** from the zone overview page.
3. **Zitadel bootstrap credential** (the one thing Terraform can't create
   is the credential it authenticates with): in the console, Users →
   Service Users → create machine user `terraform` (access token type:
   Bearer). Grant it management rights: Organization → the ⚙/Managers
   pane → add `terraform` as **Org Owner**. On the user's Personal
   Access Tokens tab, generate a **PAT** (set an expiry and calendar its
   rotation). Also copy the **organization ID** from the Organization
   page.
4. **Clear the hand-made Zitadel project**: delete the console-created
   `order-api` project (Projects → order-api → delete). Users are
   org-level and unaffected; Terraform recreates the project and apps
   with fresh client IDs that flow into the containers automatically.
   (Alternative for zero user-visible churn: `terraform import` the
   existing project and apps instead.)
5. **Run `scripts/bootstrap-github.sh`** (fills all GitHub secrets and
   variables from `scripts/bootstrap.env`). Secrets: `SCW_ACCESS_KEY`,
   `SCW_SECRET_KEY`, `SCW_PROJECT_ID`, `CLOUDFLARE_API_TOKEN`,
   `DATABASE_URL`, `AUTH_SECRET`, `ZITADEL_PAT`, `NEW_RELIC_LICENSE_KEY`.
   Variables: `SCW_REGISTRY_NAMESPACE`, `CLOUDFLARE_ZONE_ID`,
   `ZITADEL_ORG_ID`.
6. Merge to main. First apply creates the Zitadel project + apps +
   policies, the namespace, containers, DNS records, and domain
   bindings; certificate issuance takes a few minutes after the CNAMEs
   propagate. If the very first Zitadel API call times out (serverless
   cold start), just re-run the job.
7. **Decommission the hand-made containers** from the pre-Terraform setup
   once `https://orders.boozoo.top` works — delete them in the Scaleway
   console to stop paying for min-scale instances.

Still console-only, by choice: SMTP provider settings, logo/icon/font
uploads on the Branding page (binary assets don't belong in a repo), and
human admin users.

## Design decisions

- **DNS-only (gray cloud) Cloudflare records.** Scaleway terminates TLS
  with its own Let's Encrypt certificates, and issuance/renewal require
  the CNAME to be visible. Proxied orange-cloud records break that.
  Cloudflare is used here purely as the DNS host.
- **Secrets flow GitHub → TF vars → container secret env.** CI is the
  only writer; nothing is committed. Consequence: secrets exist in the
  state file, hence the private-bucket requirement. (The alternative —
  managing env by hand and `ignore_changes` — was rejected because
  "create the container" was the thing being automated.)
- **`min_scale = 1` on both containers.** A warm API instance keeps its
  pgx connection pool and Zitadel JWKS cache; a cold one pays TLS to Neon
  plus OIDC discovery on the first request. Given the latency focus, the
  always-on instance is the cheapest win available. Tune via variables.
- **Zitadel PAT with Org Owner, not instance admin.** The `terraform`
  machine user only needs to manage one org's projects, apps, and
  policies — scoping it there limits the blast radius of a leaked PAT.
  Note the login policy and branding are org-level *custom* policies:
  they override the instance defaults for the default org, which is what
  every user of this system hits.
- **No state locking** (Scaleway Object Storage lacks conditional
  writes); applies are serialized by the CI concurrency group instead.
  Avoid running `terraform apply` locally while CI is deploying.

## Local usage (rarely needed)

```bash
cd infra
export SCW_ACCESS_KEY=... SCW_SECRET_KEY=... SCW_DEFAULT_PROJECT_ID=...
export CLOUDFLARE_API_TOKEN=...
export AWS_ACCESS_KEY_ID=$SCW_ACCESS_KEY AWS_SECRET_ACCESS_KEY=$SCW_SECRET_KEY
terraform init
terraform plan -var image_tag=<some-sha> \
  -var registry_namespace=<ns> -var cloudflare_zone_id=<id> \
  -var zitadel_client_id_api=<id> -var auth_zitadel_id=<id> \
  -var zitadel_project_id=<id> -var database_url=... \
  -var auth_secret=... -var auth_zitadel_secret=...
```

## Observability: seeing Neon network latency for what it is

The Go service already emits, via OTLP:

- an **`otelhttp` server span** per request,
- **`otelpgx` client spans** per query — these measure *client-observed*
  time: network round trip to Neon + TLS + protocol + actual execution,
- **pgxpool metrics** (`RecordStats`): acquire duration, idle/used
  connections — pool wait shows up here, not inside query spans,
- **`db.client.rtt` histogram** (new): a 30s `SELECT`-nothing ping whose
  duration is almost pure network + protocol floor to Neon. The probe is
  **activity-gated**: it samples only while `/orders*` traffic occurred in
  the last 2 minutes, and goes silent when the app is idle — Neon is
  serverless and autosuspends, and the probe must never be what keeps it
  awake. Corollary: expect gaps in the metric during quiet hours (that's
  correct behavior), and read p50/p95 rather than mean — the first sample
  after a Neon resume includes the resume cost and shows as an outlier.

Point it at New Relic by setting `NEW_RELIC_LICENSE_KEY` (the stack sets
`OTEL_EXPORTER_OTLP_ENDPOINT` and the `api-key` header for you; endpoint
defaults to the EU ingest, override `new_relic_otlp_endpoint` for US
accounts).

How to read it in New Relic (APM & Services → order-api → Distributed
tracing): inside a request's waterfall, the gap between the server span
and its child query spans is Go handler time; each query span is
network + execution combined. Subtract the RTT baseline to split those:

```
query span duration − db.client.rtt (p50) ≈ true server-side execution
```

NRQL to chart the network floor against query latency:

```
SELECT percentile(db.client.rtt, 50, 95) FROM Metric TIMESERIES
SELECT average(duration.ms) FROM Span WHERE db.system = 'postgresql' FACET name TIMESERIES
```

If `db.client.rtt` p50 sits at, say, 8–12 ms (nl-ams → Neon's region) and
your query spans average 12 ms, the database is executing in ~2 ms and
the network is the story — exactly the "don't blame the database"
evidence this was built for. If instead pool acquire duration spikes,
the bottleneck is pool sizing, visible in the pgxpool metrics rather
than either network or Postgres.
