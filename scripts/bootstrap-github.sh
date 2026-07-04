#!/usr/bin/env bash
# Sets every GitHub Actions secret and variable the CI/CD pipeline needs,
# from a local (gitignored) env file, via the gh CLI. Idempotent: re-run
# any time; existing values are overwritten.
#
# Usage:
#   cp scripts/bootstrap.env.example scripts/bootstrap.env
#   $EDITOR scripts/bootstrap.env
#   ./scripts/bootstrap-github.sh [owner/repo]
#
# owner/repo is optional — inferred from the git remote when omitted.
#
# Design notes:
# - Values are passed to gh via --body from shell variables, never as
#   loose CLI args built by hand, so nothing secret lands in shell
#   history or `ps` output beyond the gh invocation itself.
# - AUTH_SECRET is auto-generated when left empty: production must not
#   share the session-encryption key with any other environment, and
#   rotating it merely signs everyone out.
# - NEW_RELIC_LICENSE_KEY is optional: when empty we still set an empty
#   secret so the workflow reference resolves, and Terraform's
#   empty-means-disabled convention keeps telemetry off.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/bootstrap.env"

# ---------- preconditions ----------
if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI not found — install from https://cli.github.com" >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh is not authenticated — run 'gh auth login' first" >&2
  exit 1
fi
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "error: ${ENV_FILE} not found." >&2
  echo "  cp ${SCRIPT_DIR}/bootstrap.env.example ${ENV_FILE}" >&2
  echo "  then fill it in and re-run. It is gitignored — keep it that way." >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a; source "${ENV_FILE}"; set +a

REPO="${1:-}"
REPO_FLAG=()
if [[ -n "${REPO}" ]]; then
  REPO_FLAG=(--repo "${REPO}")
fi

# ---------- derived / validated values ----------
if [[ -z "${AUTH_SECRET:-}" ]]; then
  AUTH_SECRET="$(openssl rand -base64 32)"
  echo "AUTH_SECRET was empty — generated a fresh one (only stored in GitHub; not written back to ${ENV_FILE})."
fi

require() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "error: ${name} is empty in ${ENV_FILE} — fill it in (see the comments there for where it comes from)." >&2
    exit 1
  fi
}

for v in SCW_ACCESS_KEY SCW_SECRET_KEY SCW_PROJECT_ID \
         CLOUDFLARE_API_TOKEN CLOUDFLARE_ZONE_ID \
         DATABASE_URL \
         ZITADEL_ORG_ID ZITADEL_PAT \
         SCW_REGISTRY_NAMESPACE; do
  require "$v"
done

# ---------- secrets ----------
set_secret() {
  local name="$1" value="$2"
  gh secret set "${name}" ${REPO_FLAG[@]+"${REPO_FLAG[@]}"} --body "${value}"
  echo "  secret   ${name}  ✓"
}

# ---------- variables (non-sensitive identifiers) ----------
set_variable() {
  local name="$1" value="$2"
  gh variable set "${name}" ${REPO_FLAG[@]+"${REPO_FLAG[@]}"} --body "${value}"
  echo "  variable ${name} = ${value}"
}

echo "Setting repository secrets…"
set_secret SCW_ACCESS_KEY        "${SCW_ACCESS_KEY}"
set_secret SCW_SECRET_KEY        "${SCW_SECRET_KEY}"
set_secret SCW_PROJECT_ID        "${SCW_PROJECT_ID}"
set_secret CLOUDFLARE_API_TOKEN  "${CLOUDFLARE_API_TOKEN}"
set_secret DATABASE_URL          "${DATABASE_URL}"
set_secret AUTH_SECRET           "${AUTH_SECRET}"
set_secret ZITADEL_PAT           "${ZITADEL_PAT}"
set_secret NEW_RELIC_LICENSE_KEY "${NEW_RELIC_LICENSE_KEY:-}"
if [[ -z "${NEW_RELIC_LICENSE_KEY:-}" ]]; then
  echo "  note: NEW_RELIC_LICENSE_KEY is empty → telemetry stays disabled (fill it in and re-run to enable)."
fi

echo "Setting repository variables…"
set_variable SCW_REGISTRY_NAMESPACE "${SCW_REGISTRY_NAMESPACE}"
set_variable CLOUDFLARE_ZONE_ID     "${CLOUDFLARE_ZONE_ID}"
set_variable ZITADEL_ORG_ID         "${ZITADEL_ORG_ID}"

# ---------- cleanup of obsolete entries from the pre-Terraform deploy ----------
for obsolete in SCW_CONTAINER_ID SCW_WEB_CONTAINER_ID \
                ZITADEL_CLIENT_ID_API AUTH_ZITADEL_ID ZITADEL_PROJECT_ID; do
  if gh variable list ${REPO_FLAG[@]+"${REPO_FLAG[@]}"} --json name --jq '.[].name' 2>/dev/null | grep -qx "${obsolete}"; then
    gh variable delete "${obsolete}" ${REPO_FLAG[@]+"${REPO_FLAG[@]}"}
    echo "  removed obsolete variable ${obsolete}"
  fi
done

if gh secret list ${REPO_FLAG[@]+"${REPO_FLAG[@]}"} --json name --jq '.[].name' 2>/dev/null | grep -qx "AUTH_ZITADEL_SECRET"; then
  gh secret delete AUTH_ZITADEL_SECRET ${REPO_FLAG[@]+"${REPO_FLAG[@]}"}
  echo "  removed obsolete secret AUTH_ZITADEL_SECRET (now Terraform-managed)"
fi

echo
echo "Final state:"
gh secret list ${REPO_FLAG[@]+"${REPO_FLAG[@]}"}
gh variable list ${REPO_FLAG[@]+"${REPO_FLAG[@]}"}
echo
echo "Done. Next: merge to main and the deploy job has everything it needs."
