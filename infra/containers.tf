# Serverless Containers.
#
# Terraform owns the containers end to end: existence, sizing, runtime
# env, secrets, and which image tag runs. CI's docker job pushes
# :<sha> images; the deploy job then applies this stack with
# -var image_tag=<sha>, and the image change triggers the
# rollout (deploy = true). This replaces the previous curl-PATCH deploy.

locals {
  registry = "rg.${var.region}.scw.cloud/${var.registry_namespace}"

  # Telemetry env is attached only when a New Relic key is provided, so
  # the app's "no endpoint → telemetry off" convention keeps working.
  otel_env = var.new_relic_license_key == "" ? {} : {
    OTEL_EXPORTER_OTLP_ENDPOINT = var.new_relic_otlp_endpoint
    OTEL_RESOURCE_ATTRIBUTES    = "deployment.environment=production"
  }
  otel_secret_env = var.new_relic_license_key == "" ? {} : {
    # The exporter sends this verbatim as an HTTP header on every OTLP
    # request — exactly what New Relic expects.
    OTEL_EXPORTER_OTLP_HEADERS = "api-key=${var.new_relic_license_key}"
  }
}

resource "scaleway_container_namespace" "main" {
  name        = "order-api"
  description = "order-api production containers (managed by Terraform)"
}

resource "scaleway_container" "api" {
  name         = "order-api"
  namespace_id = scaleway_container_namespace.main.id

  image          = "${local.registry}/order-api:${var.image_tag}"
  port           = 8080
  protocol       = "http1"
  privacy        = "public"
  http_option    = "redirected" # force https

  min_scale    = var.api_min_scale
  max_scale    = 4
  cpu_limit    = 500 # mvCPU
  memory_limit = 512 # MB

  environment_variables = merge({
    ZITADEL_DOMAIN    = var.zitadel_domain
    ZITADEL_CLIENT_ID = zitadel_application_api.backend.client_id
  }, local.otel_env)

  secret_environment_variables = merge({
    DATABASE_URL = var.database_url
  }, local.otel_secret_env)

  deploy = true
}

resource "scaleway_container" "web" {
  name         = "order-api-web"
  namespace_id = scaleway_container_namespace.main.id

  image          = "${local.registry}/order-api-web:${var.image_tag}"
  port           = 3000
  protocol       = "http1"
  privacy        = "public"
  http_option    = "redirected"

  min_scale    = var.web_min_scale
  max_scale    = 4
  cpu_limit    = 500
  memory_limit = 512

  environment_variables = {
    AUTH_ZITADEL_ISSUER = var.zitadel_domain
    AUTH_ZITADEL_ID     = zitadel_application_oidc.web.client_id
    ZITADEL_PROJECT_ID  = zitadel_project.order_api.id
    ORDER_API_URL       = "https://${var.api_hostname}"
    AUTH_URL            = "https://${var.web_hostname}"
    AUTH_TRUST_HOST     = "true"
  }

  secret_environment_variables = {
    AUTH_SECRET         = var.auth_secret
    AUTH_ZITADEL_SECRET = zitadel_application_oidc.web.client_secret
  }

  deploy = true
}
