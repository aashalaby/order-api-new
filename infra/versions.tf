# Providers and remote state.
#
# State lives in Scaleway Object Storage (S3-compatible). The bucket is
# created once, by hand (it can't manage the state it stores):
#
#   scw object bucket create order-api-tfstate region=nl-ams
#
# Credentials in CI: the backend reads AWS_ACCESS_KEY_ID /
# AWS_SECRET_ACCESS_KEY (set to the Scaleway keys), the scaleway provider
# reads SCW_ACCESS_KEY / SCW_SECRET_KEY / SCW_DEFAULT_PROJECT_ID, and the
# cloudflare provider reads CLOUDFLARE_API_TOKEN. Nothing is committed.
#
# NOTE: the state file contains the container secrets (DATABASE_URL,
# AUTH_SECRET, ...). The bucket must stay private; treat read access to it
# as production-secret access. Scaleway Object Storage does not support
# S3 conditional-write locking, so concurrency is serialized at the CI
# level instead (concurrency group on the deploy job).

terraform {
  required_version = ">= 1.9"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "~> 2.72"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
    zitadel = {
      source  = "zitadel/zitadel"
      version = "~> 3.0"
    }
  }

  backend "s3" {
    bucket = "order-api-tfstate"
    key    = "order-api/infra.tfstate"
    region = "nl-ams"
    endpoints = {
      s3 = "https://s3.nl-ams.scw.cloud"
    }
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
  }
}

provider "scaleway" {
  region = var.region
}

provider "cloudflare" {}
