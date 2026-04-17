#!/bin/bash

set -e
set -o pipefail

trap 'echo "❌ Error on line $LINENO" >&2' ERR

DRY_RUN=${DRY_RUN:-0}
DEBUG=${DEBUG:-0}
PARALLEL=${PARALLEL:-4}
MAX_RETRIES=${MAX_RETRIES:-5}

CHECKPOINT_FILE=".metaobject_checkpoint"
CATEGORY_MAP_FILE=".category_map.json"
QUEUE_FILE="/tmp/metaobject_queue.$$"

API_VERSION="2025-10"

BASE_PATH="backups/2026-04-15/metaobjects"
CATEGORY_FILE="$BASE_PATH/faq_categories.json"
ITEM_FILE="$BASE_PATH/faq_item.json"
GRADES_FILE="$BASE_PATH/grades.json"

# =========================
# ENV
# =========================
if [ -f .env ]; then
  set -o allexport
  source .env
  set +o allexport
fi

SHOPIFY_STORE=${SHOPIFY_STORE#https://}
BASE_URL="https://$SHOPIFY_STORE/admin/api/$API_VERSION"

ACCESS_TOKEN=""
TOKEN_EXPIRY=0

TOTAL=0
DONE=0
START_TIME=$(date +%s)

# =========================
# LOGGING
# =========================
log(){ echo "$@" >&2; }
debug(){ [ "$DEBUG" -eq 1 ] && echo "🐞 $@" >&2; }

progress(){
  DONE=$((DONE+1))
  NOW=$(date +%s)
  ELAPSED=$((NOW - START_TIME))
  RATE=$(( DONE > 0 ? ELAPSED / DONE : 0 ))
  REMAIN=$(( TOTAL - DONE ))
  ETA=$(( RATE * REMAIN ))

  printf "\r📊 %d/%d | ETA: %ds" "$DONE" "$TOTAL" "$ETA" >&2
}

# =========================
# TOKEN
# =========================
get_token(){
  RESP=$(curl -s -X POST "https://$SHOPIFY_STORE/admin/oauth/access_token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=client_credentials" \
    -d "client_id=$SHOPIFY_CLIENT_ID" \
    -d "client_secret=$SHOPIFY_SECRET")

  ACCESS_TOKEN=$(echo "$RESP" | jq -r '.access_token')
  EXPIRES_IN=$(echo "$RESP" | jq -r '.expires_in')

  [ -z "$ACCESS_TOKEN" ] && { log "❌ Token failed"; exit 1; }

  TOKEN_EXPIRY=$(( $(date +%s) + EXPIRES_IN - 60 ))
}

ensure_token(){
  NOW=$(date +%s)
  if [ -z "$ACCESS_TOKEN" ] || [ "$NOW" -ge "$TOKEN_EXPIRY" ]; then
    get_token
  fi
}

# =========================
# GRAPHQL (retry)
# =========================
graphql(){
  BODY=$1
  ATTEMPT=0

  while true; do
    ensure_token

    RESP=$(curl -s -X POST "$BASE_URL/graphql.json" \
      -H "X-Shopify-Access-Token: $ACCESS_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$BODY")

    echo "$RESP" | jq empty >/dev/null 2>&1 || {
      log "❌ Invalid JSON"
      echo "$RESP" >&2
      exit 1
    }

    ERRORS=$(echo "$RESP" | jq '[..|.message?]|map(select(.!=null))|length')

    if [ "$ERRORS" -eq 0 ]; then
      return 0
    fi

    ATTEMPT=$((ATTEMPT+1))
    [ "$ATTEMPT" -ge "$MAX_RETRIES" ] && {
      log "❌ Failed after retries"
      echo "$RESP" | jq >&2
      exit 1
    }

    sleep $((2 ** ATTEMPT))
  done
}

# =========================
# HELPERS
# =========================
get_existing_metaobject(){
  graphql "$(jq -n --arg t "$1" --arg h "$2" '
  {
    query:"query($t:String!,$h:String!){
      metaobjectByHandle(handle:{type:$t,handle:$h}){id}
    }",
    variables:{t:$t,h:$h}
  }')" >/dev/null

  echo "$RESP" | jq -r '.data.metaobjectByHandle.id // empty'
}

build_mutation(){
  TYPE=$1
  HANDLE=$2
  EXISTING=$3
  FIELDS=$4

  if [ -n "$EXISTING" ]; then
    jq -n --arg id "$EXISTING" --argjson f "$FIELDS" '
    {
      query:"mutation($id:ID!,$f:[MetaobjectFieldInput!]!){
        metaobjectUpdate(id:$id,metaobject:{fields:$f}){userErrors{message}}
      }",
      variables:{id:$id,f:$f}
    }'
  else
    jq -n --arg t "$TYPE" --arg h "$HANDLE" --argjson f "$FIELDS" '
    {
      query:"mutation($t:String!,$h:String!,$f:[MetaobjectFieldInput!]!){
        metaobjectCreate(metaobject:{type:$t,handle:$h,fields:$f}){
          userErrors{message}
        }
      }",
      variables:{t:$t,h:$h,f:$f}
    }'
  fi
}

# =========================
# WORKER
# =========================
worker(){
  while read -r line; do

    TYPE=$(echo "$line" | cut -d'|' -f1)
    OBJ=$(echo "$line" | cut -d'|' -f2-)

    HANDLE=$(echo "$OBJ" | jq -r '.handle')

    grep -q "$HANDLE" "$CHECKPOINT_FILE" 2>/dev/null && continue

    FIELDS=$(echo "$OBJ" | jq -c '.fields | map({
      key:.key,
      value:(.value // (.jsonValue|tostring))
    })')

    if [ "$TYPE" == "faq_item" ]; then
      FIELDS=$(echo "$FIELDS" | jq -c \
        --slurpfile map "$CATEGORY_MAP_FILE" '
        map(if .key=="category"
          then .value = ($map[0][.value])
          else . end)')
    fi

    EXISTING=$(get_existing_metaobject "$TYPE" "$HANDLE")

    BODY=$(build_mutation "$TYPE" "$HANDLE" "$EXISTING" "$FIELDS")

    [ "$DRY_RUN" -eq 1 ] || graphql "$BODY"

    echo "$HANDLE" >> "$CHECKPOINT_FILE"

    progress

  done
}

# =========================
# QUEUE
# =========================
run_pool(){
  TYPE=$1
  FILE=$2

  rm -f "$QUEUE_FILE"
  mkfifo "$QUEUE_FILE"

  for i in $(seq 1 $PARALLEL); do
    worker < "$QUEUE_FILE" &
  done

  while read -r obj; do
    echo "$TYPE|$obj"
  done < <(jq -c '.[]' "$FILE") > "$QUEUE_FILE"

  wait
  rm -f "$QUEUE_FILE"
}

# =========================
# RUN
# =========================
log "🚀 Starting restore"

> "$CHECKPOINT_FILE"
echo "{}" > "$CATEGORY_MAP_FILE"

TOTAL=$(jq length "$CATEGORY_FILE")
run_pool "faq_categories" "$CATEGORY_FILE"

# build category map
while read -r obj; do
  OLD=$(echo "$obj"|jq -r '.id')
  HANDLE=$(echo "$obj"|jq -r '.handle')
  NEW=$(get_existing_metaobject "faq_categories" "$HANDLE")

  jq --arg k "$OLD" --arg v "$NEW" \
    '. + {($k):$v}' "$CATEGORY_MAP_FILE" > tmp && mv tmp "$CATEGORY_MAP_FILE"

done < <(jq -c '.[]' "$CATEGORY_FILE")

TOTAL=$(jq length "$GRADES_FILE")
DONE=0
run_pool "grades" "$GRADES_FILE"

TOTAL=$(jq length "$ITEM_FILE")
DONE=0
run_pool "faq_item" "$ITEM_FILE"

echo ""
log "✅ Restore complete"