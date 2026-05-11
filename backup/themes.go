package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/btafoya/goshopify-backup/logger"
	"github.com/btafoya/goshopify-backup/pkg/auth"
)

const (
	themePullRetries  = 3
	themePullBaseWait = 2 * time.Second
	themeDirPerm      = 0o700
	defaultCLIVersion = "@shopify/cli@3"
)

// Theme mirrors the shopify CLI theme list JSON output.
type Theme struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Processing bool   `json:"processing"`
}

// ThemeMeta is written to themes/{id}/.meta.json after pull.
type ThemeMeta struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	UpdatedAt      string    `json:"updated_at"`
	SchemaVersion  string    `json:"schema_version"`
	OS2            bool      `json:"online_store_2"`
	CLIVersion     string    `json:"shopify_cli_version"`
	BackedUpAt     time.Time `json:"backed_up_at"`
	PullDurationMS int64     `json:"pull_duration_ms"`
}

// ThemesModule backs up Shopify themes via the shopify CLI.
type ThemesModule struct {
	name  string
	store string
	auth  *auth.Authenticator
	log   *logger.Logger
}

// NewThemesModule creates a themes backup module.
func NewThemesModule(store string, a *auth.Authenticator, log *logger.Logger) *ThemesModule {
	return &ThemesModule{
		name:  "themes",
		store: store,
		auth:  a,
		log:   log,
	}
}

// Name returns the module name.
func (m *ThemesModule) Name() string { return m.name }

// Run executes the themes backup: ensures CLI is available, fetches token,
// lists themes, pulls each theme into themes/{id}/, writes meta.json, and
// strips CLI artifacts. Per-theme failures are recorded but do not abort
// the module.
func (m *ThemesModule) Run(ctx context.Context, outputDir string) (int, int64, error) {
	themesDir := filepath.Join(outputDir, "themes")
	if err := os.MkdirAll(themesDir, themeDirPerm); err != nil {
		return 0, 0, fmt.Errorf("create themes dir: %w", err)
	}

	cliPath, cliVersion, err := ensureShopifyCLI(ctx, m.log)
	if err != nil {
		return 0, 0, fmt.Errorf("shopify CLI: %w", err)
	}

	token, err := m.auth.EnsureToken(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("auth token: %w", err)
	}

	listRaw, themes, err := listThemes(ctx, cliPath, m.store, token)
	if err != nil {
		return 0, 0, fmt.Errorf("list themes: %w", err)
	}

	if err := os.WriteFile(filepath.Join(themesDir, "themes.json"), listRaw, 0o600); err != nil {
		return 0, 0, fmt.Errorf("write themes.json: %w", err)
	}

	var totalSize int64
	var pulled int
	var firstErr error
	for _, theme := range themes {
		themeDir := filepath.Join(themesDir, fmt.Sprintf("%d", theme.ID))
		if err := os.MkdirAll(themeDir, themeDirPerm); err != nil {
			m.log.Error("Create theme dir failed", logger.LogFields{
				Module: m.name,
				Error:  err.Error(),
			})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		start := time.Now()
		if err := pullThemeWithRetry(ctx, cliPath, m.store, token, theme.ID, themeDir, m.log); err != nil {
			m.log.Error("Theme pull failed", logger.LogFields{
				Module: m.name,
				Error:  err.Error(),
			})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		pullDur := time.Since(start)

		if err := cleanupCLIArtifacts(themeDir); err != nil {
			m.log.Error("Cleanup artifacts failed", logger.LogFields{
				Module: m.name,
				Error:  err.Error(),
			})
		}

		meta := ThemeMeta{
			ID:             theme.ID,
			Name:           theme.Name,
			Role:           theme.Role,
			UpdatedAt:      theme.UpdatedAt,
			SchemaVersion:  detectSchemaVersion(themeDir),
			OS2:            isOnlineStore2(themeDir),
			CLIVersion:     cliVersion,
			BackedUpAt:     time.Now().UTC(),
			PullDurationMS: pullDur.Milliseconds(),
		}
		if err := writeThemeMeta(themeDir, meta); err != nil {
			m.log.Error("Write theme meta failed", logger.LogFields{
				Module: m.name,
				Error:  err.Error(),
			})
		}

		size, err := dirSize(themeDir)
		if err == nil {
			totalSize += size
		}
		pulled++
	}

	// Module-level error returned only if no themes succeeded.
	if pulled == 0 && firstErr != nil {
		return 0, 0, firstErr
	}
	return pulled, totalSize, nil
}

// ensureShopifyCLI returns the path to the shopify CLI, installing it via
// npm on Linux hosts if missing. Returns the resolved CLI version string.
func ensureShopifyCLI(ctx context.Context, log *logger.Logger) (string, string, error) {
	if path, err := exec.LookPath("shopify"); err == nil {
		ver := cliVersionString(ctx, path)
		return path, ver, nil
	}

	if runtime.GOOS != "linux" {
		return "", "", fmt.Errorf("shopify CLI not found in PATH; install manually: npm install -g @shopify/cli (auto-install is Linux-only)")
	}

	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return "", "", fmt.Errorf("npm not found in PATH; install Node.js then retry: %w", err)
	}

	version := os.Getenv("SHOPIFY_CLI_VERSION")
	if version == "" {
		version = defaultCLIVersion
	}

	log.Info("Installing Shopify CLI", logger.LogFields{
		Module: "themes",
	})

	cmd := exec.CommandContext(ctx, npmPath, "install", "-g", version)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("npm install -g %s failed: %w", version, err)
	}

	path, err := exec.LookPath("shopify")
	if err != nil {
		return "", "", fmt.Errorf("shopify CLI still not found after npm install: %w", err)
	}
	ver := cliVersionString(ctx, path)
	return path, ver, nil
}

// cliVersionString returns the shopify CLI version, or "unknown" on error.
func cliVersionString(ctx context.Context, cliPath string) string {
	cmd := exec.CommandContext(ctx, cliPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// listThemes invokes `shopify theme list --json` and returns raw JSON + parsed themes.
func listThemes(ctx context.Context, cliPath, store, token string) ([]byte, []Theme, error) {
	args := []string{"theme", "list", "--json", "--store", store, "--no-color"}
	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = withShopifyToken(os.Environ(), token)

	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return nil, nil, fmt.Errorf("shopify theme list failed: %w (stderr: %s)", err, stderr)
	}

	jsonStart := indexJSONStart(out)
	if jsonStart > 0 {
		out = out[jsonStart:]
	}

	var themes []Theme
	if err := json.Unmarshal(out, &themes); err != nil {
		return nil, nil, fmt.Errorf("parse theme list: %w (raw: %s)", err, string(out))
	}
	return out, themes, nil
}

// pullThemeWithRetry invokes `shopify theme pull` with exponential backoff retry.
func pullThemeWithRetry(ctx context.Context, cliPath, store, token string, themeID int64, dest string, log *logger.Logger) error {
	delay := themePullBaseWait
	var lastErr error
	for attempt := 1; attempt <= themePullRetries; attempt++ {
		err := pullTheme(ctx, cliPath, store, token, themeID, dest)
		if err == nil {
			return nil
		}
		lastErr = err
		log.Error("Theme pull attempt failed", logger.LogFields{
			Module: "themes",
			Error:  err.Error(),
		})
		if attempt == themePullRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return lastErr
}

// pullTheme invokes a single `shopify theme pull` command.
func pullTheme(ctx context.Context, cliPath, store, token string, themeID int64, dest string) error {
	args := []string{
		"theme", "pull",
		"--theme", fmt.Sprintf("%d", themeID),
		"--store", store,
		"--path", dest,
		"--no-color",
	}
	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = withShopifyToken(os.Environ(), token)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shopify theme pull %d: %w", themeID, err)
	}
	return nil
}

// withShopifyToken adds Shopify CLI token env vars, keeping the token out of argv.
// Both env vars are set for compatibility across CLI versions.
func withShopifyToken(env []string, token string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "SHOPIFY_CLI_THEME_TOKEN=") || strings.HasPrefix(e, "SHOPIFY_FLAG_PASSWORD=") {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered,
		"SHOPIFY_CLI_THEME_TOKEN="+token,
		"SHOPIFY_FLAG_PASSWORD="+token,
	)
	return filtered
}

// cleanupCLIArtifacts removes CLI-managed metadata that should not be part of
// the canonical theme backup: .shopify/ dir and *.toml files at the top level.
func cleanupCLIArtifacts(themeDir string) error {
	if err := os.RemoveAll(filepath.Join(themeDir, ".shopify")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove .shopify: %w", err)
	}
	entries, err := os.ReadDir(themeDir)
	if err != nil {
		return fmt.Errorf("read theme dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".toml") {
			if err := os.Remove(filepath.Join(themeDir, name)); err != nil {
				return fmt.Errorf("remove %s: %w", name, err)
			}
		}
	}
	return nil
}

// detectSchemaVersion reads config/settings_schema.json and returns the theme's
// reported version, or "unknown" if absent/unreadable.
func detectSchemaVersion(themeDir string) string {
	data, err := os.ReadFile(filepath.Join(themeDir, "config", "settings_schema.json"))
	if err != nil {
		return "unknown"
	}
	var schema []map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return "unknown"
	}
	for _, entry := range schema {
		if name, ok := entry["name"].(string); ok && name == "theme_info" {
			if v, ok := entry["theme_version"].(string); ok {
				return v
			}
		}
	}
	return "unknown"
}

// isOnlineStore2 reports whether the theme uses Online Store 2.0 JSON templates.
// Heuristic: at least one .json file in templates/.
func isOnlineStore2(themeDir string) bool {
	templatesDir := filepath.Join(themeDir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return true
		}
	}
	return false
}

// writeThemeMeta serializes ThemeMeta to themes/{id}/.meta.json.
func writeThemeMeta(themeDir string, meta ThemeMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return os.WriteFile(filepath.Join(themeDir, ".meta.json"), data, 0o600)
}

// dirSize walks a directory and returns the cumulative file size in bytes.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// indexJSONStart finds the first '[' or '{' byte in CLI output, skipping
// any banner text the shopify CLI may print before JSON when stdin is not a TTY.
func indexJSONStart(b []byte) int {
	for i, c := range b {
		if c == '[' || c == '{' {
			return i
		}
	}
	return 0
}
