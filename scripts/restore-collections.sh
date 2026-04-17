#!/usr/bin/env bash
set -euo pipefail

# restore-collections.sh — Restore collections from backup
# Usage: ./scripts/restore-collections.sh [--dry-run]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_FILE="$PROJECT_DIR/backups/2026-04-15/collections.json"

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
collections=$(cat "$BACKUP_FILE")
count=$(echo "$collections" | jq 'length')
echo "=== Restoring Collections ($count total) ==="

for i in $(seq 0 $((count - 1))); do
  title=$(echo "$collections" | jq -r ".[$i].title")
  handle=$(echo "$collections" | jq -r ".[$i].handle")
  description=$(echo "$collections" | jq -r ".[$i].description // empty")
  sort_order=$(echo "$collections" | jq -r ".[$i].sortOrder")
  ruleset=$(echo "$collections" | jq ".[$i].ruleSet")

  if $DRY_RUN; then
    echo "[DRY-RUN] Would create collection: $title ($handle)"
    if [[ "$ruleset" != "null" ]]; then
      echo "  Smart collection with rules"
    else
      echo "  Manual collection"
    fi
    continue
  fi

  # Check if collection with this handle already exists
  existing=$(graphql '{ collections(first: 1, query: "'"handle:$handle"'") { nodes { id title handle } } }')
  existing_id=$(echo "$existing" | jq -r '.data.collections.nodes | if length > 0 then .[0].id else "null" end')

  if [[ "$existing_id" != "null" ]]; then
    echo "SKIP: Collection '$title' already exists ($existing_id)"
    continue
  fi

  # Build the input
  # For smart collections (with ruleSet), include rules
  # For manual collections, just title/handle/description
  if [[ "$ruleset" != "null" ]]; then
    # Build rule set input
    rules_input=$(echo "$ruleset" | jq '{
      appliedDisjunctively: false,
      rules: [.rules[] | {
        column: .column,
        relation: .relation,
        condition: .condition
      }]
    }')

    variables=$(jq -n \
      --arg title "$title" \
      --arg handle "$handle" \
      --arg desc "$description" \
      --arg sort "$sort_order" \
      --argjson rules "$rules_input" \
      '{
        input: {
          title: $title,
          handle: $handle,
          descriptionHtml: $desc,
          sortOrder: $sort,
          ruleSet: $rules
        }
      }')
  else
    variables=$(jq -n \
      --arg title "$title" \
      --arg handle "$handle" \
      --arg desc "$description" \
      --arg sort "$sort_order" \
      '{
        input: {
          title: $title,
          handle: $handle,
          descriptionHtml: $desc,
          sortOrder: $sort
        }
      }')
  fi

  result=$(graphql "mutation CollectionCreate(\$input: CollectionInput!) {
    collectionCreate(input: \$input) {
      collection { id title handle }
      userErrors { field message }
    }
  }" "$variables")

  new_id=$(echo "$result" | jq -r '.data.collectionCreate.collection.id // empty')
  errors=$(echo "$result" | jq '.data.collectionCreate.userErrors')

  if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
    echo "ERROR creating '$title': $errors"
  else
    echo "Created: $title ($handle) -> $new_id"
  fi

  sleep 0.5
done

echo ""
echo "=== Collections restore complete ==="