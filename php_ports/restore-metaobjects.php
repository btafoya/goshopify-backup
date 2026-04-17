<?php

define('ROOT_PATH', dirname(__FILE__) . DIRECTORY_SEPARATOR);
define('PROJECT_ROOT', dirname(__FILE__, 2));

require_once ROOT_PATH . 'vendor/autoload.php';

use Shopify\Context;
use Shopify\Clients\Graphql;
use Shopify\Auth\FileSessionStorage;
use GuzzleHttp\Client as GuzzleClient;

// ============================================================
// Constants
// ============================================================

const API_VERSION = '2025-01';
const TOKEN_REFRESH_BUFFER = 60;
const MIN_REQUEST_INTERVAL_US = 500000;
const MAX_RETRIES = 3;
const RETRY_BASE_DELAY_US = 2000000;
const CHECKPOINT_FILE = '.metaobject_restore_checkpoint.json';
const GID_MAP_FILE = '.category_map.json';

// ============================================================
// CLI Argument Parsing
// ============================================================

function parseArgs(array $argv): array
{
    $opts = ['dry_run' => false, 'reset' => false, 'date' => null];
    foreach (array_slice($argv, 1) as $arg) {
        if ($arg === '--dry-run') $opts['dry_run'] = true;
        elseif ($arg === '--reset') $opts['reset'] = true;
        elseif (preg_match('/^--date=(.+)$/', $arg, $m)) $opts['date'] = $m[1];
    }
    return $opts;
}

// ============================================================
// Logging
// ============================================================

function logMsg(string $msg): void  { fwrite(STDERR, $msg . PHP_EOL); }
function logErr(string $msg): void  { fwrite(STDERR, "[ERROR] $msg" . PHP_EOL); }
function logOk(string $msg): void   { fwrite(STDERR, "[OK] $msg" . PHP_EOL); }
function logSkip(string $msg): void { fwrite(STDERR, "[SKIP] $msg" . PHP_EOL); }

function progress(int $done, int $total, float $start): void
{
    $elapsed = microtime(true) - $start;
    $rate = $done > 0 ? $elapsed / $done : 0;
    $remain = $rate * ($total - $done);
    fwrite(STDERR, sprintf("\r  %d/%d | ETA: %ds", $done, $total, (int)$remain));
}

// ============================================================
// OAuth Token Management
// ============================================================

function fetchAccessToken(string $storeUrl, string $clientId, string $secret): array
{
    $guzzle = new GuzzleClient();
    $resp = $guzzle->post(rtrim($storeUrl, '/') . '/admin/oauth/access_token', [
        'form_params' => [
            'grant_type' => 'client_credentials',
            'client_id' => $clientId,
            'client_secret' => $secret,
        ],
    ]);
    $body = json_decode($resp->getBody()->getContents(), true);
    if (empty($body['access_token'])) {
        throw new RuntimeException('OAuth token exchange failed: ' . json_encode($body));
    }
    return $body;
}

function getAccessToken(string $storeUrl, string $clientId, string $secret): string
{
    $cacheFile = '/tmp/shopify_restore_token_' . md5($clientId) . '.json';
    if (file_exists($cacheFile)) {
        $cached = json_decode(file_get_contents($cacheFile), true);
        if (!empty($cached['access_token']) && ($cached['expires_at'] ?? 0) > time() + TOKEN_REFRESH_BUFFER) {
            return $cached['access_token'];
        }
    }
    $body = fetchAccessToken($storeUrl, $clientId, $secret);
    $expiresIn = (int)($body['expires_in'] ?? 86399);
    $cache = [
        'access_token' => $body['access_token'],
        'expires_at' => time() + $expiresIn - TOKEN_REFRESH_BUFFER,
    ];
    file_put_contents($cacheFile, json_encode($cache));
    return $body['access_token'];
}

function ensureFreshClient(Graphql &$client, string $shopDomain, string $storeUrl, string $clientId, string $secret): void
{
    $cacheFile = '/tmp/shopify_restore_token_' . md5($clientId) . '.json';
    if (!file_exists($cacheFile)) return;
    $cached = json_decode(file_get_contents($cacheFile), true);
    if (($cached['expires_at'] ?? 0) <= time() + TOKEN_REFRESH_BUFFER) {
        $token = getAccessToken($storeUrl, $clientId, $secret);
        $client = new Graphql($shopDomain, $token);
    }
}

// ============================================================
// GraphQL Client Wrapper
// ============================================================

function graphql(Graphql $client, string $query, array $variables, bool $dryRun): array
{
    if ($dryRun) {
        logMsg("  [DRY-RUN] " . substr($query, 0, 60) . '...');
        return ['data' => []];
    }

    $attempt = 0;
    while (true) {
        try {
            $response = $client->query(
                ['query' => $query, 'variables' => $variables],
                [],
                [],
                MAX_RETRIES
            );
            usleep(MIN_REQUEST_INTERVAL_US);

            $body = $response->getDecodedBody();

            if (!empty($body['errors'])) {
                $msgs = array_column($body['errors'], 'message');
                throw new RuntimeException('GraphQL errors: ' . implode('; ', $msgs));
            }

            return $body;
        } catch (Psr\Http\Client\ClientExceptionInterface $e) {
            $attempt++;
            if ($attempt >= MAX_RETRIES) throw $e;
            usleep(RETRY_BASE_DELAY_US * (1 << $attempt));
        } catch (Shopify\Exception\HttpRequestException $e) {
            $attempt++;
            if ($attempt >= MAX_RETRIES) throw $e;
            usleep(RETRY_BASE_DELAY_US * (1 << $attempt));
        }
    }
}

function checkUserErrors(array $body, string $key): bool
{
    $errors = $body['data'][$key]['userErrors'] ?? [];
    if (!empty($errors)) {
        foreach ($errors as $e) {
            logErr(sprintf('%s: %s', implode('.', $e['field'] ?? []), $e['message']));
        }
        return true;
    }
    return false;
}

// ============================================================
// Checkpoint Management
// ============================================================

function loadCheckpoint(string $path): array
{
    if (!file_exists($path)) return [];
    $data = json_decode(file_get_contents($path), true);
    return is_array($data) ? $data : [];
}

function saveCheckpoint(string $path, array $cp): void
{
    $tmp = $path . '.tmp';
    file_put_contents($tmp, json_encode($cp, JSON_PRETTY_PRINT));
    rename($tmp, $path);
}

function isCompleted(array $cp, string $phase, string $handle): bool
{
    return in_array($handle, $cp[$phase] ?? [], true);
}

function markCompleted(array &$cp, string $phase, string $handle): void
{
    $cp[$phase] = array_values(array_unique(array_merge($cp[$phase] ?? [], [$handle])));
}

// ============================================================
// GID Map (old GID -> new GID for metaobject_reference fields)
// ============================================================

function loadGidMap(string $path): array
{
    if (!file_exists($path)) return [];
    $data = json_decode(file_get_contents($path), true);
    return is_array($data) ? $data : [];
}

function saveGidMap(string $path, array $map): void
{
    $tmp = $path . '.tmp';
    file_put_contents($tmp, json_encode($map, JSON_PRETTY_PRINT));
    rename($tmp, $path);
}

// ============================================================
// Phase 1: Definition Creation
// ============================================================

const Q_FIND_DEFINITION = <<<'GQL'
query findDefinition($type: String!) {
  metaobjectDefinitionByType(type: $type) {
    id
    type
    capabilities {
      publishable { enabled }
      translatable { enabled }
    }
  }
}
GQL;

const M_CREATE_DEFINITION = <<<'GQL'
mutation createDefinition($definition: MetaobjectDefinitionCreateInput!) {
  metaobjectDefinitionCreate(definition: $definition) {
    metaobjectDefinition {
      id
      type
    }
    userErrors { field message code }
  }
}
GQL;

const M_UPDATE_DEFINITION = <<<'GQL'
mutation updateDefinition($id: ID!, $definition: MetaobjectDefinitionUpdateInput!) {
  metaobjectDefinitionUpdate(id: $id, definition: $definition) {
    metaobjectDefinition {
      id
      type
      capabilities {
        publishable { enabled }
      }
    }
    userErrors { field message code }
  }
}
GQL;

const REFERENCE_MAP = [
    'faq_item' => ['category' => 'faq_categories'],
];

function buildDefinitionInput(array $def, array $definitionGids): array
{
    $fieldDefs = [];
    foreach ($def['fieldDefinitions'] as $fd) {
        $input = [
            'key' => $fd['key'],
            'name' => $fd['name'],
            'type' => $fd['type']['name'],
        ];
        if ($fd['type']['name'] === 'metaobject_reference') {
            $refType = REFERENCE_MAP[$def['type']][$fd['key']] ?? null;
            if ($refType && isset($definitionGids[$refType])) {
                $input['validations'] = [
                    ['name' => 'metaobject_definition_id', 'value' => $definitionGids[$refType]],
                ];
            }
        }
        $fieldDefs[] = $input;
    }
    return [
        'name' => $def['name'],
        'type' => $def['type'],
        'fieldDefinitions' => $fieldDefs,
        'capabilities' => ['publishable' => ['enabled' => true]],
    ];
}

function createDefinitions(Graphql $client, array $definitions, array &$definitionGids, array &$checkpoint, bool $dryRun): void
{
    // Order: no-dep definitions first, then definitions with metaobject_reference deps
    $noDeps = [];
    $hasDeps = [];
    foreach ($definitions as $def) {
        $hasRef = false;
        foreach ($def['fieldDefinitions'] as $fd) {
            if ($fd['type']['name'] === 'metaobject_reference') { $hasRef = true; break; }
        }
        if ($hasRef) $hasDeps[] = $def; else $noDeps[] = $def;
    }
    $ordered = array_merge($noDeps, $hasDeps);

    foreach ($ordered as $def) {
        $type = $def['type'];

        if (isCompleted($checkpoint, 'definitions', $type) && isset($definitionGids[$type])) {
            logSkip("Definition '$type' already in checkpoint");
            continue;
        }

        // Check if definition already exists on the store
        $resp = graphql($client, Q_FIND_DEFINITION, ['type' => $type], $dryRun);
        $existingId = $resp['data']['metaobjectDefinitionByType']['id'] ?? null;

        if ($existingId) {
            $definitionGids[$type] = $existingId;

            // Enable publishable capability if not already enabled
            $publishable = $resp['data']['metaobjectDefinitionByType']['capabilities']['publishable']['enabled'] ?? false;
            if (!$publishable && !$dryRun) {
                logMsg("  Enabling publishable on '$type'...");
                $updateResp = graphql($client, M_UPDATE_DEFINITION, [
                    'id' => $existingId,
                    'definition' => ['capabilities' => ['publishable' => ['enabled' => true]]],
                ], $dryRun);
                if (checkUserErrors($updateResp, 'metaobjectDefinitionUpdate')) {
                    logErr("Failed to enable publishable on '$type'");
                } else {
                    logOk("Publishable enabled on '$type'");
                }
            }

            logSkip("Definition '$type' exists ($existingId)");
            markCompleted($checkpoint, 'definitions', $type);
            continue;
        }

        $input = buildDefinitionInput($def, $definitionGids);
        $resp = graphql($client, M_CREATE_DEFINITION, ['definition' => $input], $dryRun);

        if (!$dryRun && checkUserErrors($resp, 'metaobjectDefinitionCreate')) {
            logErr("Failed to create definition '$type'");
            continue;
        }

        $newId = $resp['data']['metaobjectDefinitionCreate']['metaobjectDefinition']['id'] ?? 'dry-run';
        $definitionGids[$type] = $newId;
        markCompleted($checkpoint, 'definitions', $type);
        logOk("Definition '$type' -> $newId");
    }
}

// ============================================================
// Phase 2: Entry Creation
// ============================================================

const Q_FIND_ENTRY = <<<'GQL'
query findEntry($type: String!, $handle: String!) {
  metaobjectByHandle(handle: {type: $type, handle: $handle}) {
    id
    handle
    fields { key value }
  }
}
GQL;

const M_CREATE_ENTRY = <<<'GQL'
mutation createEntry($metaobject: MetaobjectCreateInput!) {
  metaobjectCreate(metaobject: $metaobject) {
    metaobject { id handle type }
    userErrors { field message code }
  }
}
GQL;

const M_UPDATE_ENTRY = <<<'GQL'
mutation updateEntry($id: ID!, $metaobject: MetaobjectUpdateInput!) {
  metaobjectUpdate(id: $id, metaobject: $metaobject) {
    metaobject { id handle }
    userErrors { field message code }
  }
}
GQL;

function buildFieldValues(array $fields, array $gidMap): array
{
    $result = [];
    foreach ($fields as $f) {
        $value = (string)$f['value'];
        if ($f['type'] === 'metaobject_reference' && isset($gidMap[$value])) {
            $value = $gidMap[$value];
        }
        $result[] = ['key' => $f['key'], 'value' => $value];
    }
    return $result;
}

function createEntries(
    Graphql $client,
    string $type,
    array $entries,
    array &$gidMap,
    array &$checkpoint,
    bool $dryRun,
    float $startTime
): int
{
    $total = count($entries);
    $done = 0;

    foreach ($entries as $entry) {
        $handle = $entry['handle'];

        if (isCompleted($checkpoint, $type, $handle)) {
            logSkip("$type/$handle (checkpoint)");
            $done++;
            progress($done, $total, $startTime);
            continue;
        }

        // Check if entry already exists
        $resp = graphql($client, Q_FIND_ENTRY, ['type' => $type, 'handle' => $handle], $dryRun);
        $existing = $resp['data']['metaobjectByHandle'] ?? null;

        if ($existing && !$dryRun) {
            logSkip("$type/$handle (exists, GID: {$existing['id']})");
            $gidMap[$entry['id']] = $existing['id'];
            markCompleted($checkpoint, $type, $handle);
            $done++;
            progress($done, $total, $startTime);
            continue;
        }

        $fields = buildFieldValues($entry['fields'], $gidMap);
        $input = [
            'type' => $type,
            'handle' => $handle,
            'fields' => $fields,
        ];

        // Set publishable capability if present in backup
        $status = $entry['capabilities']['publishable']['status'] ?? null;
        if ($status) {
            $input['capabilities'] = ['publishable' => ['status' => $status]];
        }

        $resp = graphql($client, M_CREATE_ENTRY, ['metaobject' => $input], $dryRun);

        if (!$dryRun && checkUserErrors($resp, 'metaobjectCreate')) {
            logErr("Failed to create $type/$handle");
            $done++;
            progress($done, $total, $startTime);
            continue;
        }

        $newId = $resp['data']['metaobjectCreate']['metaobject']['id'] ?? 'dry-run';
        $gidMap[$entry['id']] = $newId;
        markCompleted($checkpoint, $type, $handle);
        logOk("$type/$handle -> $newId");

        $done++;
        progress($done, $total, $startTime);
    }

    return $done;
}

// ============================================================
// Phase 3: Cross-Reference Verification
// ============================================================

function updateCrossReferences(
    Graphql $client,
    array $faqItemEntries,
    array $gidMap,
    array &$checkpoint,
    bool $dryRun
): void
{
    $updated = 0;
    foreach ($faqItemEntries as $entry) {
        $handle = $entry['handle'];
        if (isCompleted($checkpoint, 'crossrefs', $handle)) continue;

        // Find the category field from backup
        $catField = null;
        foreach ($entry['fields'] as $f) {
            if ($f['key'] === 'category' && $f['type'] === 'metaobject_reference') {
                $catField = $f;
                break;
            }
        }
        if (!$catField) continue;

        $oldGid = $catField['value'];
        $newGid = $gidMap[$oldGid] ?? null;
        if (!$newGid) {
            logErr("No GID mapping for category $oldGid in $handle");
            continue;
        }

        // Query current entry state
        $resp = graphql($client, Q_FIND_ENTRY, ['type' => 'faq_item', 'handle' => $handle], $dryRun);
        $existing = $resp['data']['metaobjectByHandle'] ?? null;
        if (!$existing) continue;

        // Check if category field already has the correct GID
        $currentCatGid = null;
        foreach ($existing['fields'] as $f) {
            if ($f['key'] === 'category') { $currentCatGid = $f['value']; break; }
        }

        if ($currentCatGid === $newGid) {
            markCompleted($checkpoint, 'crossrefs', $handle);
            continue;
        }

        // Update the category field
        $resp = graphql($client, M_UPDATE_ENTRY, [
            'id' => $existing['id'],
            'metaobject' => ['fields' => [['key' => 'category', 'value' => $newGid]]],
        ], $dryRun);

        if (!$dryRun && checkUserErrors($resp, 'metaobjectUpdate')) {
            logErr("Failed to update category for $handle");
            continue;
        }

        markCompleted($checkpoint, 'crossrefs', $handle);
        logOk("Updated category ref: $handle -> $newGid");
        $updated++;
    }
    if ($updated > 0) logMsg("Updated $updated cross-references");
}

// ============================================================
// Main
// ============================================================

function main(array $argv): void
{
    $opts = parseArgs($argv);
    $startTime = microtime(true);

    // Load environment
    $dotenv = Dotenv\Dotenv::createImmutable(PROJECT_ROOT);
    $dotenv->load();

    $storeUrl = $_ENV['SHOPIFY_STORE'] ?? '';
    $clientId = $_ENV['SHOPIFY_CLIENT_ID'] ?? '';
    $secret = $_ENV['SHOPIFY_SECRET'] ?? '';

    if (!$storeUrl || !$clientId || !$secret) {
        logErr('Missing SHOPIFY_STORE, SHOPIFY_CLIENT_ID, or SHOPIFY_SECRET in .env');
        exit(1);
    }

    // Initialize SDK context (required by Graphql client internals)
    Context::initialize(
        apiKey: $clientId,
        apiSecretKey: $secret,
        scopes: 'write_metaobjects,read_metaobjects',
        hostName: $storeUrl,
        sessionStorage: new FileSessionStorage('/tmp/php_sessions'),
        apiVersion: API_VERSION,
        isEmbeddedApp: false,
        isPrivateApp: false,
    );

    // Get access token and create client
    $token = getAccessToken($storeUrl, $clientId, $secret);
    $shopDomain = parse_url($storeUrl, PHP_URL_HOST) ?: $storeUrl;
    $client = new Graphql($shopDomain, $token);

    // Resolve backup directory
    $backupBase = $_ENV['BACKUP_DIR'] ?? PROJECT_ROOT . '/backups';
    $date = $opts['date'];
    if (!$date) {
        $dirs = glob($backupBase . '/20*');
        $date = $dirs ? basename(end($dirs)) : null;
    }
    if (!$date) {
        logErr('No backup date found. Use --date=YYYY-MM-DD');
        exit(1);
    }
    $backupDir = $backupBase . '/' . $date . '/metaobjects';
    if (!is_dir($backupDir)) {
        logErr("Backup directory not found: $backupDir");
        exit(1);
    }

    // Load checkpoint
    $checkpointPath = PROJECT_ROOT . '/' . CHECKPOINT_FILE;
    $checkpoint = $opts['reset'] ? [] : loadCheckpoint($checkpointPath);

    // Load GID map
    $gidMapPath = PROJECT_ROOT . '/' . GID_MAP_FILE;
    $gidMap = loadGidMap($gidMapPath);

    // Load backup data
    $defsFile = $backupDir . '/metaobject-definitions.json';
    $definitions = json_decode(file_get_contents($defsFile), true);

    $entryFiles = ['faq_categories', 'grades', 'faq_item'];
    $entryData = [];
    foreach ($entryFiles as $type) {
        $file = $backupDir . '/' . $type . '.json';
        $entryData[$type] = file_exists($file) ? json_decode(file_get_contents($file), true) : [];
    }

    logMsg('Restoring metaobjects from ' . $backupDir);

    // === PHASE 1: Create definitions ===
    logMsg('Phase 1: Ensuring metaobject definitions...');
    $definitionGids = [];
    createDefinitions($client, $definitions, $definitionGids, $checkpoint, $opts['dry_run']);
    if (!$opts['dry_run']) saveCheckpoint($checkpointPath, $checkpoint);

    // === PHASE 2: Create entries ===
    logMsg('Phase 2: Restoring metaobject entries...');
    $totalDone = 0;
    foreach ($entryFiles as $type) {
        if (empty($entryData[$type])) continue;
        $count = count($entryData[$type]);
        logMsg("  $type: $count entries");
        ensureFreshClient($client, $shopDomain, $storeUrl, $clientId, $secret);
        $totalDone += createEntries($client, $type, $entryData[$type], $gidMap, $checkpoint, $opts['dry_run'], $startTime);
        if (!$opts['dry_run']) {
            saveCheckpoint($checkpointPath, $checkpoint);
            saveGidMap($gidMapPath, $gidMap);
        }
        fwrite(STDERR, PHP_EOL);
    }

    // === PHASE 3: Verify cross-references ===
    logMsg('Phase 3: Verifying cross-references...');
    ensureFreshClient($client, $shopDomain, $storeUrl, $clientId, $secret);
    updateCrossReferences($client, $entryData['faq_item'] ?? [], $gidMap, $checkpoint, $opts['dry_run']);
    if (!$opts['dry_run']) saveCheckpoint($checkpointPath, $checkpoint);

    $elapsed = round(microtime(true) - $startTime, 1);
    logMsg("Done in {$elapsed}s — $totalDone entries processed");
}

main($argv);