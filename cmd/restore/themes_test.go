package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

// writeRestoreFakeCLI writes a fake shopify CLI for restore tests. Behavior is
// driven by env vars set per test (FAKE_PUSH_FAIL_TIMES, FAKE_PUSH_THEME_ID).
func writeRestoreFakeCLI(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI requires unix")
	}
	script := `#!/bin/sh
set -e

case "$1" in
  version) echo "fake-cli/0.0.1"; exit 0 ;;
esac

# Verify token came via env, not --password flag
for arg in "$@"; do
  case "$arg" in
    --password=*|--password)
      echo "ERROR: token passed via --password flag (should be env var)" >&2
      exit 99
      ;;
  esac
done

if [ -z "$SHOPIFY_CLI_THEME_TOKEN" ] && [ -z "$SHOPIFY_FLAG_PASSWORD" ]; then
  echo "ERROR: no token env var set" >&2
  exit 98
fi

case "$1" in
  theme)
    case "$2" in
      push)
        # Verify --unpublished was passed
        seen_unpublished=0
        seen_json=0
        path=""
        shift 2
        while [ $# -gt 0 ]; do
          case "$1" in
            --unpublished) seen_unpublished=1; shift ;;
            --json) seen_json=1; shift ;;
            --path) path="$2"; shift 2 ;;
            *) shift ;;
          esac
        done
        if [ "$seen_unpublished" -ne 1 ]; then
          echo "ERROR: --unpublished not passed" >&2
          exit 50
        fi
        if [ "$seen_json" -ne 1 ]; then
          echo "ERROR: --json not passed" >&2
          exit 51
        fi
        if [ -z "$path" ]; then
          echo "ERROR: --path not passed" >&2
          exit 52
        fi

        # Simulate transient failure when FAKE_PUSH_FAIL_TIMES > 0
        if [ -n "$FAKE_PUSH_FAIL_TIMES" ] && [ -f "$path/.push_attempts" ]; then
          attempts=$(cat "$path/.push_attempts")
          if [ "$attempts" -lt "$FAKE_PUSH_FAIL_TIMES" ]; then
            echo $((attempts + 1)) > "$path/.push_attempts"
            echo "transient push failure" >&2
            exit 1
          fi
        elif [ -n "$FAKE_PUSH_FAIL_TIMES" ]; then
          echo "1" > "$path/.push_attempts"
          echo "transient push failure" >&2
          exit 1
        fi

        new_id="${FAKE_PUSH_THEME_ID:-987654321}"
        rm -f "$path/.push_attempts"
        cat <<JSON
{"theme":{"id":${new_id},"name":"Restored","role":"unpublished","shop":"test.myshopify.com","editor_url":"https://test.myshopify.com/admin/themes/${new_id}/editor","preview_url":"https://test.myshopify.com/?preview_theme_id=${new_id}"}}
JSON
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

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", orig) })
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+orig); err != nil {
		t.Fatal(err)
	}
}

// newTestClient builds a minimal ShopifyClient suitable for unit tests of
// the theme restorer. The HTTPClient is unused for CLI calls.
func newTestClient(t *testing.T, store, token string) *ShopifyClient {
	t.Helper()
	return &ShopifyClient{
		StoreURL:    store,
		AccessToken: token,
		APIVersion:  "2025-01",
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
		RateLimiter: NewRateLimiter(40),
		logger:      log.New(os.Stderr),
	}
}

func TestThemeRestorer_EnsureCLI_Found(t *testing.T) {
	dir := t.TempDir()
	writeRestoreFakeCLI(t, dir)
	prependPATH(t, dir)

	client := newTestClient(t, "test.myshopify.com", "tok")
	r := NewThemeRestorer(client, log.New(os.Stderr))

	path, err := r.ensureCLI(context.Background())
	if err != nil {
		t.Fatalf("ensureCLI: %v", err)
	}
	if !strings.HasSuffix(path, "/shopify") {
		t.Errorf("unexpected CLI path: %s", path)
	}
	// Second call should reuse cached path
	path2, err := r.ensureCLI(context.Background())
	if err != nil || path != path2 {
		t.Errorf("expected cached path; got %s, err=%v", path2, err)
	}
}

func TestThemeRestorer_EnsureCLI_NonLinuxMissing(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux-only behavior")
	}
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	r := NewThemeRestorer(newTestClient(t, "x.myshopify.com", "tok"), log.New(os.Stderr))
	_, err := r.ensureCLI(context.Background())
	if err == nil || !strings.Contains(err.Error(), "install manually") {
		t.Errorf("want manual install error, got: %v", err)
	}
}

func TestThemeRestorer_RestoreTheme_DryRun(t *testing.T) {
	client := newTestClient(t, "test.myshopify.com", "tok")
	client.DryRun = true
	r := NewThemeRestorer(client, log.New(os.Stderr))

	id, err := r.RestoreTheme(context.Background(), t.TempDir(), "Dawn")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if id != "" {
		t.Errorf("dry-run should return empty ID, got %q", id)
	}
}

func TestThemeRestorer_RestoreTheme_MissingPath(t *testing.T) {
	cliDir := t.TempDir()
	writeRestoreFakeCLI(t, cliDir)
	prependPATH(t, cliDir)

	client := newTestClient(t, "test.myshopify.com", "tok")
	r := NewThemeRestorer(client, log.New(os.Stderr))

	_, err := r.RestoreTheme(context.Background(), "/no/such/dir", "")
	if err == nil {
		t.Fatal("expected error for missing theme path")
	}
}

func TestThemeRestorer_RestoreTheme_Success(t *testing.T) {
	cliDir := t.TempDir()
	writeRestoreFakeCLI(t, cliDir)
	prependPATH(t, cliDir)

	themeDir := t.TempDir()
	// Need at least one file so stat succeeds + path is well-formed
	if err := os.WriteFile(filepath.Join(themeDir, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_PUSH_THEME_ID", "1122334455")

	client := newTestClient(t, "test.myshopify.com", "tok-abc")
	r := NewThemeRestorer(client, log.New(os.Stderr))

	gid, err := r.RestoreTheme(context.Background(), themeDir, "Restored Dawn")
	if err != nil {
		t.Fatalf("RestoreTheme: %v", err)
	}
	want := "gid://shopify/OnlineStoreTheme/1122334455"
	if gid != want {
		t.Errorf("want %s, got %s", want, gid)
	}
}

func TestThemeRestorer_RestoreTheme_RetryRecovers(t *testing.T) {
	cliDir := t.TempDir()
	writeRestoreFakeCLI(t, cliDir)
	prependPATH(t, cliDir)

	themeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(themeDir, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_PUSH_FAIL_TIMES", "2") // fail twice, succeed on attempt 3
	t.Setenv("FAKE_PUSH_THEME_ID", "777")

	client := newTestClient(t, "test.myshopify.com", "tok")
	r := NewThemeRestorer(client, log.New(os.Stderr))

	gid, err := r.RestoreTheme(context.Background(), themeDir, "")
	if err != nil {
		t.Fatalf("RestoreTheme should recover: %v", err)
	}
	if !strings.HasSuffix(gid, "/777") {
		t.Errorf("unexpected gid: %s", gid)
	}
}

func TestWithRestoreShopifyToken_ReplacesStale(t *testing.T) {
	env := []string{"FOO=bar", "SHOPIFY_CLI_THEME_TOKEN=old", "SHOPIFY_FLAG_PASSWORD=stale", "KEEP=me"}
	out := withRestoreShopifyToken(env, "fresh")

	hasTheme, hasFlag, hasFoo, hasKeep := false, false, false, false
	for _, e := range out {
		switch {
		case e == "SHOPIFY_CLI_THEME_TOKEN=fresh":
			hasTheme = true
		case e == "SHOPIFY_FLAG_PASSWORD=fresh":
			hasFlag = true
		case e == "FOO=bar":
			hasFoo = true
		case e == "KEEP=me":
			hasKeep = true
		case strings.HasPrefix(e, "SHOPIFY_CLI_THEME_TOKEN=") && e != "SHOPIFY_CLI_THEME_TOKEN=fresh":
			t.Errorf("stale theme token survived: %s", e)
		case strings.HasPrefix(e, "SHOPIFY_FLAG_PASSWORD=") && e != "SHOPIFY_FLAG_PASSWORD=fresh":
			t.Errorf("stale flag password survived: %s", e)
		}
	}
	if !hasTheme || !hasFlag {
		t.Errorf("missing fresh tokens: theme=%v flag=%v", hasTheme, hasFlag)
	}
	if !hasFoo || !hasKeep {
		t.Errorf("unrelated env vars dropped: foo=%v keep=%v", hasFoo, hasKeep)
	}
}

func TestTrimToJSONStart(t *testing.T) {
	cases := map[string]string{
		`{"theme":{}}`:               `{"theme":{}}`,
		"banner\n{\"theme\":{}}":     `{"theme":{}}`,
		"banner\n[1,2,3]":            "[1,2,3]",
		"no json":                    "no json",
	}
	for in, want := range cases {
		got := string(trimToJSONStart([]byte(in)))
		if got != want {
			t.Errorf("trimToJSONStart(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExecutor_RollbackScriptForThemes(t *testing.T) {
	exec := &RestoreExecutor{
		client: newTestClient(t, "test.myshopify.com", "tok"),
		rollbackActions: []RollbackAction{
			{
				EntityType:  EntityThemes,
				Action:      "delete",
				ID:          "gid://shopify/OnlineStoreTheme/12345",
				Description: "Delete restored theme: My Theme",
			},
		},
		logger: log.New(os.Stderr),
	}

	dir := t.TempDir()
	scriptPath, err := exec.WriteRollbackScript(dir, "2026-05-11")
	if err != nil {
		t.Fatalf("WriteRollbackScript: %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "shopify theme delete --force --theme 12345") {
		t.Errorf("rollback script missing CLI delete command:\n%s", body)
	}
	if !strings.Contains(body, "SHOPIFY_CLI_THEME_TOKEN=") {
		t.Errorf("rollback script should pass token via env var:\n%s", body)
	}
}
