#!/usr/bin/env bash
set -euo pipefail

# restore-pages.sh — Restore pages from backup
# Usage: ./scripts/restore-pages.sh [--dry-run]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_FILE="$PROJECT_DIR/backups/2026-04-15/pages.json"

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
pages=$(cat "$BACKUP_FILE")
count=$(echo "$pages" | jq 'length')
echo "=== Restoring Pages ($count total) ==="

for i in $(seq 0 $((count - 1))); do
  title=$(echo "$pages" | jq -r ".[$i].title")
  handle=$(echo "$pages" | jq -r ".[$i].handle")
  body_html=$(echo "$pages" | jq -r ".[$i].body_html // empty")
  template_suffix=$(echo "$pages" | jq -r ".[$i].template_suffix // empty")
  published_at=$(echo "$pages" | jq -r ".[$i].published_at // empty")

  if $DRY_RUN; then
    echo "[DRY-RUN] Would create page: $title ($handle)"
    [[ -n "$template_suffix" ]] && echo "  template: $template_suffix"
    continue
  fi

  # Check if page with this handle already exists
  existing=$(graphql '{ pages(first: 1, query: "'"handle:$handle"'") { nodes { id title handle } } }')
  existing_id=$(echo "$existing" | jq -r '.data.pages.nodes | if length > 0 then .[0].id else "null" end')

  if [[ "$existing_id" != "null" ]]; then
    echo "SKIP: Page '$title' already exists ($existing_id)"
    continue
  fi

  # Build variables - escape body_html for JSON
  is_published="true"
  [[ "$published_at" == "null" || -z "$published_at" ]] && is_published="false"

  # Build input JSON
  input_obj=$(jq -n \
    --arg title "$title" \
    --arg handle "$handle" \
    --arg body "$body_html" \
    --argjson isPub "$is_published" \
    '{
      title: $title,
      handle: $handle,
      body: $body,
      isPublished: $isPub
    }')

  # Add template_suffix if present
  if [[ -n "$template_suffix" && "$template_suffix" != "null" ]]; then
    input_obj=$(echo "$input_obj" | jq --arg ts "$template_suffix" '. + {templateSuffix: $ts}')
  fi

  variables=$(jq -n --argjson input "$input_obj" '{page: $input}')

  result=$(graphql "mutation CreatePage(\$page: PageCreateInput!) {
    pageCreate(page: \$page) {
      page { id title handle }
      userErrors { field message code }
    }
  }" "$variables")

  new_id=$(echo "$result" | jq -r '.data.pageCreate.page.id // empty')
  errors=$(echo "$result" | jq '.data.pageCreate.userErrors')

  if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
    echo "ERROR creating '$title': $errors"
  else
    echo "Created: $title ($handle) -> $new_id"
  fi

  sleep 0.5
done

echo ""
echo "=== Pages restore complete ==="