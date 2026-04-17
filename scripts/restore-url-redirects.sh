#!/usr/bin/env bash
set -euo pipefail

# restore-url-redirects.sh — Restore URL redirects from backup
# Usage: ./scripts/restore-url-redirects.sh [--dry-run]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_FILE="$PROJECT_DIR/backups/2026-04-15/url-redirects.json"

# Load .env
set -a; source "$PROJECT_DIR/.env"; set +a

API_VERSION="2026-01"
STORE="${SHOPIFY_STORE}"
DRY_RUN=false
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

# --- Get access token ---
get_token() {
  if [[ -n "${SHOPIFY_ACCESS_TOKEN:-}" ]]; then
    echo "$SHOPIFY_ACCESS_TOKEN"
    return
  fi
  local token response
  response=$(curl -s -X POST \
    "${STORE}/admin/oauth/access_token" \
    -d "grant_type=client_credentials" \
    -d "client_id=${SHOPIFY_CLIENT_ID}" \
    -d "client_secret=${SHOPIFY_SECRET}")
  token=$(echo "$response" | jq -r '.access_token')
  if [[ "$token" == "null" || -z "$token" ]]; then
    echo "ERROR: Failed to obtain access token" >&2
    echo "$response" | jq . >&2
    exit 1
  fi
  echo "$token"
}

graphql() {
  local query="$1" variables="${2:-}"
  local payload
  payload=$(jq -n --arg q "$query" --argjson v "${variables:-{}}" '{query: $q, variables: $v}')
  curl -s -X POST \
    "${STORE}/admin/api/${API_VERSION}/graphql.json" \
    -H "Content-Type: application/json" \
    -H "X-Shopify-Access-Token: ${ACCESS_TOKEN}" \
    -d "$payload"
}

ACCESS_TOKEN=$(get_token)

# --- Load backup data ---
redirects=$(cat "$BACKUP_FILE")
count=$(echo "$redirects" | jq 'length')
echo "=== Restoring URL Redirects ($count total) ==="

created=0
skipped=0
errors=0

for i in $(seq 0 $((count - 1))); do
  path=$(echo "$redirects" | jq -r ".[$i].path")
  target=$(echo "$redirects" | jq -r ".[$i].target")

  if $DRY_RUN; then
    echo "[DRY-RUN] Would create redirect: $path -> $target"
    continue
  fi

  variables=$(jq -n --arg path "$path" --arg target "$target" '{
    urlRedirect: { path: $path, target: $target }
  }')

  result=$(graphql "mutation UrlRedirectCreate(\$urlRedirect: UrlRedirectInput!) {
    urlRedirectCreate(urlRedirect: \$urlRedirect) {
      urlRedirect { id path target }
      userErrors { field message }
    }
  }" "$variables")

  user_errors=$(echo "$result" | jq '.data.urlRedirectCreate.userErrors')

  if [[ "$(echo "$user_errors" | jq 'length')" -gt 0 ]]; then
    error_msg=$(echo "$user_errors" | jq -r '.[0].message')
    echo "ERROR: $path -> $target : $error_msg"
    ((errors++))
  else
    new_id=$(echo "$result" | jq -r '.data.urlRedirectCreate.urlRedirect.id // empty')
    echo "Created: $path -> $target ($new_id)"
    ((created++))
  fi

  sleep 0.5
done

echo ""
echo "=== URL Redirects restore complete: $created created, $skipped skipped, $errors errors ==="