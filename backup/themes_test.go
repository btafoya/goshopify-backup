package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/btafoya/goshopify-backup/logger"
)

// writeFakeCLI writes an executable shell script at dir/shopify that mimics
// the shopify CLI for testing. Behavior is driven by env vars set by tests
// (FAKE_LIST_JSON, FAKE_PULL_FAIL_TIMES, FAKE_PULL_LAYOUT).
func writeFakeCLI(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI shell script requires unix")
	}
	script := `#!/bin/sh
set -e

case "$1" in
  version)
    echo "fake-cli/0.0.1"
    exit 0
    ;;
esac

# Verify token came via env var (not argv) — fail if --password flag is present
for arg in "$@"; do
  case "$arg" in
    --password=*|--password)
      echo "ERROR: token passed via --password flag (should be env var)" >&2
      exit 99
      ;;
  esac
done

# Require at least one of the documented token env vars (skipped for version above)
if [ -z "$SHOPIFY_CLI_THEME_TOKEN" ] && [ -z "$SHOPIFY_FLAG_PASSWORD" ]; then
  echo "ERROR: no token env var set" >&2
  exit 98
fi

case "$1" in
  theme)
    case "$2" in
      list)
        echo "${FAKE_LIST_JSON:-[]}"
        ;;
      pull)
        # Parse --path and --theme from args
        dest=""
        theme=""
        shift 2
        while [ $# -gt 0 ]; do
          case "$1" in
            --path) dest="$2"; shift 2 ;;
            --theme) theme="$2"; shift 2 ;;
            *) shift ;;
          esac
        done

        # Simulate transient failure for retry tests
        if [ -n "$FAKE_PULL_FAIL_TIMES" ] && [ -f "$dest/.attempts" ]; then
          attempts=$(cat "$dest/.attempts")
          if [ "$attempts" -lt "$FAKE_PULL_FAIL_TIMES" ]; then
            echo $((attempts + 1)) > "$dest/.attempts"
            echo "transient failure" >&2
            exit 1
          fi
        elif [ -n "$FAKE_PULL_FAIL_TIMES" ]; then
          echo "1" > "$dest/.attempts"
          echo "transient failure" >&2
          exit 1
        fi

        # Write canonical theme structure
        mkdir -p "$dest/layout" "$dest/templates" "$dest/sections" "$dest/snippets" "$dest/assets" "$dest/config" "$dest/locales"
        echo "<!doctype html>" > "$dest/layout/theme.liquid"
        echo "{}" > "$dest/templates/index.json"
        echo '[{"name":"theme_info","theme_version":"2.0"}]' > "$dest/config/settings_schema.json"

        # Write CLI artifacts that should be cleaned up
        mkdir -p "$dest/.shopify"
        echo "cache" > "$dest/.shopify/cache"
        echo "[environments]" > "$dest/shopify.theme.toml"

        # Remove attempts marker
        rm -f "$dest/.attempts"
        ;;
      *)
        echo "unknown theme subcommand: $2" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    echo "unknown command: $1" >&2
    exit 2
    ;;
esac
`
	path := filepath.Join(dir, "shopify")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake CLI: %v", err)
	}
	return path
}

// withPATH prepends dir to PATH for the test's duration.
func withPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", orig) })
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+orig); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureShopifyCLI_Found(t *testing.T) {
	cliDir := t.TempDir()
	writeFakeCLI(t, cliDir)
	withPATH(t, cliDir)

	log := logger.NewLogger("error")
	path, ver, err := ensureShopifyCLI(context.Background(), log)
	if err != nil {
		t.Fatalf("ensureShopifyCLI: %v", err)
	}
	if !strings.HasSuffix(path, "/shopify") {
		t.Errorf("unexpected path: %s", path)
	}
	if ver != "fake-cli/0.0.1" {
		t.Errorf("unexpected version: %s", ver)
	}
}

func TestEnsureShopifyCLI_MissingNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux behavior test")
	}
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	log := logger.NewLogger("error")
	_, _, err := ensureShopifyCLI(context.Background(), log)
	if err == nil {
		t.Fatal("expected error on missing CLI for non-Linux host")
	}
	if !strings.Contains(err.Error(), "install manually") {
		t.Errorf("error should mention manual install: %v", err)
	}
}

func TestWithShopifyToken_SetsBothEnvVars(t *testing.T) {
	env := []string{"FOO=bar", "SHOPIFY_CLI_THEME_TOKEN=old", "SHOPIFY_FLAG_PASSWORD=stale"}
	out := withShopifyToken(env, "new-token")

	foundTheme, foundFlag, sawFoo := false, false, false
	for _, e := range out {
		switch {
		case e == "SHOPIFY_CLI_THEME_TOKEN=new-token":
			foundTheme = true
		case e == "SHOPIFY_FLAG_PASSWORD=new-token":
			foundFlag = true
		case e == "FOO=bar":
			sawFoo = true
		case strings.HasPrefix(e, "SHOPIFY_CLI_THEME_TOKEN=") && e != "SHOPIFY_CLI_THEME_TOKEN=new-token":
			t.Errorf("stale theme token survived: %s", e)
		case strings.HasPrefix(e, "SHOPIFY_FLAG_PASSWORD=") && e != "SHOPIFY_FLAG_PASSWORD=new-token":
			t.Errorf("stale flag password survived: %s", e)
		}
	}
	if !foundTheme || !foundFlag {
		t.Errorf("missing tokens: theme=%v flag=%v", foundTheme, foundFlag)
	}
	if !sawFoo {
		t.Error("unrelated env var dropped")
	}
}

func TestListThemes_ParsesJSON(t *testing.T) {
	cliDir := t.TempDir()
	cliPath := writeFakeCLI(t, cliDir)
	t.Setenv("FAKE_LIST_JSON", `[{"id":1,"name":"Dawn","role":"main","updated_at":"2026-01-01T00:00:00Z"},{"id":2,"name":"Refresh","role":"unpublished","updated_at":"2026-01-02T00:00:00Z"}]`)

	raw, themes, err := listThemes(context.Background(), cliPath, "test.myshopify.com", "tok-123")
	if err != nil {
		t.Fatalf("listThemes: %v", err)
	}
	if len(themes) != 2 {
		t.Fatalf("want 2 themes, got %d", len(themes))
	}
	if themes[0].Name != "Dawn" || themes[0].Role != "main" {
		t.Errorf("theme[0] mismatch: %+v", themes[0])
	}
	if !strings.Contains(string(raw), "Dawn") {
		t.Error("raw JSON missing expected content")
	}
}

func TestPullTheme_SuccessAndCleanup(t *testing.T) {
	cliDir := t.TempDir()
	cliPath := writeFakeCLI(t, cliDir)
	destDir := t.TempDir()

	if err := pullTheme(context.Background(), cliPath, "test.myshopify.com", "tok-x", 42, destDir); err != nil {
		t.Fatalf("pullTheme: %v", err)
	}

	// Theme files present
	if _, err := os.Stat(filepath.Join(destDir, "layout", "theme.liquid")); err != nil {
		t.Errorf("missing layout/theme.liquid: %v", err)
	}

	// Cleanup removes .shopify/ and *.toml
	if err := cleanupCLIArtifacts(destDir); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, ".shopify")); !os.IsNotExist(err) {
		t.Error(".shopify dir should be removed")
	}
	if _, err := os.Stat(filepath.Join(destDir, "shopify.theme.toml")); !os.IsNotExist(err) {
		t.Error("shopify.theme.toml should be removed")
	}
}

func TestPullThemeWithRetry_RecoversAfterTransientFailure(t *testing.T) {
	cliDir := t.TempDir()
	cliPath := writeFakeCLI(t, cliDir)
	t.Setenv("FAKE_PULL_FAIL_TIMES", "2") // fail twice, succeed on attempt 3
	destDir := t.TempDir()

	log := logger.NewLogger("error")
	err := pullThemeWithRetry(context.Background(), cliPath, "test.myshopify.com", "tok", 1, destDir, log)
	if err != nil {
		t.Fatalf("pullThemeWithRetry should recover, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "layout", "theme.liquid")); err != nil {
		t.Errorf("missing theme.liquid after recovery: %v", err)
	}
}

func TestDetectSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `[{"name":"theme_info","theme_version":"15.3.0"},{"name":"colors","settings":[]}]`
	if err := os.WriteFile(filepath.Join(dir, "config", "settings_schema.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectSchemaVersion(dir); got != "15.3.0" {
		t.Errorf("want 15.3.0, got %q", got)
	}
}

func TestDetectSchemaVersion_Missing(t *testing.T) {
	dir := t.TempDir()
	if got := detectSchemaVersion(dir); got != "unknown" {
		t.Errorf("want unknown, got %q", got)
	}
}

func TestIsOnlineStore2_DetectsJSONTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isOnlineStore2(dir) {
		t.Error("expected OS2 detection from index.json")
	}
}

func TestIsOnlineStore2_LegacyLiquidOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "index.liquid"), []byte("{{ content_for_index }}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isOnlineStore2(dir) {
		t.Error("legacy theme should not be flagged as OS2")
	}
}

func TestWriteThemeMeta_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	meta := ThemeMeta{
		ID:            12345,
		Name:          "Dawn",
		Role:          "main",
		SchemaVersion: "15.0.0",
		OS2:           true,
		CLIVersion:    "fake-cli/0.0.1",
		BackedUpAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := writeThemeMeta(dir, meta); err != nil {
		t.Fatalf("writeThemeMeta: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got ThemeMeta
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != meta.ID || got.Role != meta.Role || got.OS2 != meta.OS2 {
		t.Errorf("meta roundtrip mismatch: %+v vs %+v", meta, got)
	}
}

func TestIndexJSONStart(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"clean JSON", `[{"id":1}]`, 0},
		{"banner before array", "Loading...\n[]", 11},
		{"banner before object", "Initializing\n{\"x\":1}", 13},
		{"no JSON", "no json here", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := indexJSONStart([]byte(tc.in))
			if got != tc.want {
				t.Errorf("want %d, got %d", tc.want, got)
			}
		})
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	files := map[string]int{
		"a.txt":         100,
		"sub/b.txt":     250,
		"sub/sub2/c.js": 50,
	}
	for path, size := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := dirSize(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 400 {
		t.Errorf("want 400 bytes, got %d", got)
	}
}

func TestCleanupCLIArtifacts_HandlesMissingDir(t *testing.T) {
	dir := t.TempDir()
	// No .shopify/, no *.toml present — should be no-op without error.
	if err := cleanupCLIArtifacts(dir); err != nil {
		t.Errorf("cleanup on empty dir should not error: %v", err)
	}
}

func TestCleanupCLIArtifacts_RemovesMultipleToml(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"shopify.theme.toml", "extra.toml", "keep.liquid"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupCLIArtifacts(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.liquid")); err != nil {
		t.Error("non-toml file should survive cleanup")
	}
	for _, name := range []string{"shopify.theme.toml", "extra.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", name)
		}
	}
}

func TestCLIVersionString_FallbackOnError(t *testing.T) {
	// Use /bin/false as a CLI path that always exits non-zero.
	got := cliVersionString(context.Background(), "/bin/false")
	if got != "unknown" {
		t.Errorf("want unknown, got %q", got)
	}
}

// fmt unused import guard
var _ = fmt.Sprintf
