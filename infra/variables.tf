variable "region" {
  description = "Scaleway region for all serverless resources"
  type        = string
  default     = "nl-ams"
}

variable "registry_namespace" {
  description = "Scaleway Container Registry namespace the CI docker job pushes to (same value as the SCW_REGISTRY_NAMESPACE repo variable)"
  type        = string
}

variable "image_tag" {
  description = "Image tag to deploy — CI passes the commit SHA. A changed tag is what triggers a rollout."
  type        = string
}

# ------------------------------------------------------------------
# Domains (DNS zone boozoo.top is hosted on Cloudflare)
# ------------------------------------------------------------------

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID of boozoo.top (Cloudflare dashboard → zone overview, right column)"
  type        = string
}

variable "web_hostname" {
  description = "Public hostname of the Next.js app"
  type        = string
  default     = "orders.boozoo.top"
}

variable "api_hostname" {
  description = "Public hostname of the Go API"
  type        = string
  default     = "api.boozoo.top"
}

# ------------------------------------------------------------------
# Zitadel (IDs are not sensitive; the web client secret is)
# ------------------------------------------------------------------

variable "zitadel_domain" {
  description = "Base URL of the self-hosted Zitadel instance"
  type        = string
  default     = "https://zitadelc8ba5db8-zitadel.functions.fnc.nl-ams.scw.cloud"
}

variable "zitadel_org_id" {
  description = "ID of the (default) Zitadel organization the project, apps, and policies live in — Organization page → copy ID"
  type        = string
}

variable "zitadel_pat" {
  description = "Personal Access Token of the 'terraform' machine user (Org Owner). The only Zitadel credential not managed here, because Terraform can't create the credential it authenticates with."
  type        = string
  sensitive   = true
}

# ------------------------------------------------------------------
# Runtime secrets
# ------------------------------------------------------------------

variable "database_url" {
  description = "Neon Postgres connection string for the Go API"
  type        = string
  sensitive   = true
}

variable "auth_secret" {
  description = "Auth.js session-cookie encryption key for the web app"
  type        = string
  sensitive   = true
}

# ------------------------------------------------------------------
# Observability (New Relic via OTLP). Leave the key empty to run with
# telemetry fully disabled — the app is no-op without the endpoint env.
# ------------------------------------------------------------------

variable "new_relic_license_key" {
  description = "New Relic ingest license key. Empty string disables telemetry entirely."
  type        = string
  sensitive   = true
  default     = ""
}

variable "new_relic_otlp_endpoint" {
  description = "New Relic OTLP endpoint. EU accounts: https://otlp.eu01.nr-data.net:4318 — US accounts: https://otlp.nr-data.net:4318"
  type        = string
  default     = "https://otlp.eu01.nr-data.net:4318"
}

# ------------------------------------------------------------------
# Scaling
# ------------------------------------------------------------------

variable "api_min_scale" {
  description = "Minimum instances for the API. Keep at 1: a warm instance holds its pgx pool and JWKS cache, which is the single cheapest latency win."
  type        = number
  default     = 1
}

variable "web_min_scale" {
  description = "Minimum instances for the web app. 1 avoids Next.js cold starts on first visit."
  type        = number
  default     = 1
}
