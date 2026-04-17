#!/usr/bin/env bash
set -euo pipefail

# restore-metaobjects.sh — Restore metaobject definitions and entries from backup
# Usage: ./scripts/restore-metaobjects.sh [--dry-run]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="$PROJECT_DIR/backups/2026-04-15/metaobjects"

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

# --- Step 1: Create metaobject definitions ---
echo "=== Restoring Metaobject Definitions ==="
definitions=$(cat "$BACKUP_DIR/metaobject-definitions.json")
def_count=$(echo "$definitions" | jq 'length')
echo "Found $def_count definitions to create"

# Track created definition types to new IDs
declare -A DEF_IDS

for i in $(seq 0 $((def_count - 1))); do
  name=$(echo "$definitions" | jq -r ".[$i].name")
  type=$(echo "$definitions" | jq -r ".[$i].type")
  fields=$(echo "$definitions" | jq ".[$i].fieldDefinitions")

  # Build field definitions input
  field_inputs=$(echo "$fields" | jq '[
    .[] | {
      name: .name,
      key: .key,
      type: .type.name
    }
  ]')

  if $DRY_RUN; then
    echo "[DRY-RUN] Would create definition: $name ($type)"
    echo "  Fields: $(echo "$field_inputs" | jq -c '.')"
    continue
  fi

  # Check if definition already exists
  existing=$(graphql '{ metaobjectDefinitions(first: 10, query: "type:'"$type"'") { nodes { id type name } } }')
  existing_id=$(echo "$existing" | jq -r '.data.metaobjectDefinitions.nodes | if length > 0 then .[0].id else "null" end')

  if [[ "$existing_id" != "null" ]]; then
    echo "Definition '$type' already exists ($existing_id), skipping"
    DEF_IDS["$type"]="$existing_id"
    continue
  fi

  result=$(graphql "mutation CreateDef(\$definition: MetaobjectDefinitionCreateInput!) {
    metaobjectDefinitionCreate(definition: \$definition) {
      metaobjectDefinition { id name type }
      userErrors { field message code }
    }
  }" "$(jq -n --arg name "$name" --arg type "$type" --argjson fields "$field_inputs" '{
    definition: { name: $name, type: $type, fieldDefinitions: $fields }
  }')")

  new_id=$(echo "$result" | jq -r '.data.metaobjectDefinitionCreate.metaobjectDefinition.id // empty')
  errors=$(echo "$result" | jq '.data.metaobjectDefinitionCreate.userErrors')

  if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
    echo "ERROR creating definition '$type': $errors"
  else
    echo "Created definition: $name ($type) -> $new_id"
    DEF_IDS["$type"]="$new_id"
  fi

  sleep 0.5
done

# --- Step 2: Create metaobject entries ---
echo ""
echo "=== Restoring Metaobject Entries ==="

# Process each metaobject type file (skip definitions file)
for file in "$BACKUP_DIR"/faq_categories.json "$BACKUP_DIR"/faq_item.json "$BACKUP_DIR"/grades.json; do
  [[ ! -f "$file" ]] && continue
  basename_file=$(basename "$file")
  type_key="${basename_file%.json}"

  entries=$(cat "$file")
  count=$(echo "$entries" | jq 'length')
  echo ""
  echo "--- Type: $type_key ($count entries) ---"

  # Build old-to-new handle map for metaobject_reference resolution
  # We'll need this for faq_item -> faq_categories references
  declare -A HANDLE_MAP

  for i in $(seq 0 $((count - 1))); do
    handle=$(echo "$entries" | jq -r ".[$i].handle")
    mtype=$(echo "$entries" | jq -r ".[$i].type")
    fields_raw=$(echo "$entries" | jq ".[$i].fields")
    capabilities=$(echo "$entries" | jq -r ".[$i].capabilities.publishable.status // empty")

    # Build fields input - resolve metaobject_reference GIDs
    field_inputs=$(echo "$fields_raw" | jq '[
      .[] | {
        key: .key,
        value: (if .type == "rich_text_field" then (.jsonValue | tostring) else (.value | tostring) end)
      }
    ]')

    if $DRY_RUN; then
      echo "[DRY-RUN] Would create: $mtype / $handle"
      continue
    fi

    # Build capabilities input
    cap_input=""
    if [[ -n "$capabilities" ]]; then
      cap_input=$(jq -n --arg status "$capabilities" '{publishable: {status: $status}}')
    fi

    # Create the metaobject
    variables=$(jq -n \
      --arg type "$mtype" \
      --arg handle "$handle" \
      --argjson fields "$field_inputs" \
      --argjson cap "${cap_input:-{}}" '
      {
        metaobject: {
          type: $type,
          handle: $handle,
          fields: $fields
        } + (if ($cap | keys | length) > 0 then {capabilities: $cap} else {} end)
      }
    ')

    result=$(graphql "mutation CreateMO(\$metaobject: MetaobjectCreateInput!) {
      metaobjectCreate(metaobject: \$metaobject) {
        metaobject { id handle type }
        userErrors { field message code }
      }
    }" "$variables")

    new_id=$(echo "$result" | jq -r '.data.metaobjectCreate.metaobject.id // empty')
    errors=$(echo "$result" | jq '.data.metaobjectCreate.userErrors')

    if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
      echo "ERROR creating '$mtype/$handle': $errors"
    else
      echo "Created: $mtype / $handle -> $new_id"
      HANDLE_MAP["$handle"]="$new_id"
    fi

    sleep 0.5
  done

  # Update metaobject_reference fields now that we have the new IDs
  # For faq_item, the 'category' field references faq_categories
  if [[ "$type_key" == "faq_item" ]]; then
    echo ""
    echo "--- Updating faq_item category references ---"
    # Re-read faq_categories to build old-GID -> new-GID map
    cat_entries=$(cat "$BACKUP_DIR/faq_categories.json")
    declare -A CAT_GID_MAP
    for i in $(seq 0 $(($(echo "$cat_entries" | jq 'length') - 1))); do
      old_id=$(echo "$cat_entries" | jq -r ".[$i].id")
      old_handle=$(echo "$cat_entries" | jq -r ".[$i].handle")
      if [[ -n "${HANDLE_MAP[$old_handle]:-}" ]]; then
        CAT_GID_MAP["$old_id"]="${HANDLE_MAP[$old_handle]}"
      fi
    done

    # Now update each faq_item's category field
    faq_entries=$(cat "$BACKUP_DIR/faq_item.json")
    faq_count=$(echo "$faq_entries" | jq 'length')
    for i in $(seq 0 $((faq_count - 1))); do
      handle=$(echo "$faq_entries" | jq -r ".[$i].handle")
      old_cat_gid=$(echo "$faq_entries" | jq -r ".[$i].fields[] | select(.key==\"category\") | .value")
      new_cat_gid="${CAT_GID_MAP[$old_cat_gid]:-}"

      if [[ -z "$new_cat_gid" || "$new_cat_gid" == "" ]]; then
        echo "SKIP: No mapping for category $old_cat_gid in faq_item/$handle"
        continue
      fi

      mo_id="${HANDLE_MAP[$handle]:-}"
      if [[ -z "$mo_id" ]]; then
        echo "SKIP: faq_item/$handle not created"
        continue
      fi

      if $DRY_RUN; then
        echo "[DRY-RUN] Would update $handle category: $old_cat_gid -> $new_cat_gid"
        continue
      fi

      result=$(graphql "mutation UpdateMO(\$id: ID!, \$metaobject: MetaobjectUpdateInput!) {
        metaobjectUpdate(id: \$id, metaobject: \$metaobject) {
          metaobject { id handle }
          userErrors { field message code }
        }
      }" "$(jq -n --arg id "$mo_id" --arg cat_gid "$new_cat_gid" '{
        id: $id,
        metaobject: { fields: [{ key: "category", value: $cat_gid }] }
      }')")

      errors=$(echo "$result" | jq '.data.metaobjectUpdate.userErrors')
      if [[ "$(echo "$errors" | jq 'length')" -gt 0 ]]; then
        echo "ERROR updating category for $handle: $errors"
      else
        echo "Updated category ref: $handle -> $new_cat_gid"
      fi
      sleep 0.5
    done
  fi

  unset HANDLE_MAP
done

echo ""
echo "=== Metaobject restore complete ==="