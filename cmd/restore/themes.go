package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

const (
	themeRestoreCLIDefault = "@shopify/cli@3"
	themePushRetries       = 3
	themePushBaseWait      = 2 * time.Second
)

// ThemePushResult mirrors the JSON output of `shopify theme push --json`.
type ThemePushResult struct {
	Theme struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Role       string `json:"role"`
		Shop       string `json:"shop"`
		EditorURL  string `json:"editor_url"`
		PreviewURL string `json:"preview_url"`
	} `json:"theme"`
}

// ThemeRestorer pushes a backed-up theme directory to Shopify via the CLI.
// Creates a new unpublished theme; never overwrites an existing theme.
type ThemeRestorer struct {
	client   *ShopifyClient
	logger   *log.Logger
	cliPath  string
	cliMu    sync.Mutex
	cliReady bool
}

// NewThemeRestorer constructs a ThemeRestorer bound to the given ShopifyClient.
func NewThemeRestorer(client *ShopifyClient, logger *log.Logger) *ThemeRestorer {
	if logger == nil {
		logger = log.New(os.Stderr)
	}
	return &ThemeRestorer{client: client, logger: logger}
}

// ensureCLI resolves the shopify CLI path, installing via npm on Linux if missing.
func (r *ThemeRestorer) ensureCLI(ctx context.Context) (string, error) {
	r.cliMu.Lock()
	defer r.cliMu.Unlock()

	if r.cliReady && r.cliPath != "" {
		return r.cliPath, nil
	}

	if path, err := exec.LookPath("shopify"); err == nil {
		r.cliPath = path
		r.cliReady = true
		return path, nil
	}

	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("shopify CLI not found in PATH; install manually: npm install -g @shopify/cli (auto-install is Linux-only)")
	}

	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found; install Node.js then retry: %w", err)
	}

	version := os.Getenv("SHOPIFY_CLI_VERSION")
	if version == "" {
		version = themeRestoreCLIDefault
	}
	r.logger.Infof("Installing Shopify CLI (%s)...", version)

	cmd := exec.CommandContext(ctx, npm, "install", "-g", version)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("npm install -g %s: %w", version, err)
	}

	path, err := exec.LookPath("shopify")
	if err != nil {
		return "", fmt.Errorf("shopify CLI still missing after npm install: %w", err)
	}
	r.cliPath = path
	r.cliReady = true
	return path, nil
}

// RestoreTheme pushes the given local theme directory to Shopify as a new
// unpublished theme. Returns the new theme ID as a GID-like string for
// rollback tracking.
//
// Auth: token from client.currentToken (env-based, never argv).
// Publish strategy: always --unpublished. Live promotion is a manual step
// in Shopify admin to prevent accidental publishes.
func (r *ThemeRestorer) RestoreTheme(ctx context.Context, themePath, themeName string) (string, error) {
	if r.client.DryRun {
		r.logger.Infof("[dry-run] Would push theme from %s as unpublished", themePath)
		return "", nil
	}

	if _, err := os.Stat(themePath); err != nil {
		return "", fmt.Errorf("theme path: %w", err)
	}

	cliPath, err := r.ensureCLI(ctx)
	if err != nil {
		return "", fmt.Errorf("cli: %w", err)
	}

	token, err := r.client.currentToken(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	delay := themePushBaseWait
	var lastErr error
	for attempt := 1; attempt <= themePushRetries; attempt++ {
		id, err := r.pushOnce(ctx, cliPath, themePath, themeName, token)
		if err == nil {
			r.logger.Infof("Theme pushed: new ID=%d source=%s", id, themePath)
			return fmt.Sprintf("gid://shopify/OnlineStoreTheme/%d", id), nil
		}
		lastErr = err
		r.logger.Errorf("Theme push attempt %d failed: %v", attempt, err)
		if attempt == themePushRetries {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return "", lastErr
}

// pushOnce runs a single `shopify theme push --unpublished --json --path …` command.
func (r *ThemeRestorer) pushOnce(ctx context.Context, cliPath, themePath, themeName, token string) (int64, error) {
	args := []string{
		"theme", "push",
		"--unpublished",
		"--json",
		"--path", themePath,
		"--store", r.client.StoreURL,
		"--no-color",
	}
	if themeName != "" {
		args = append(args, "--theme-name", themeName)
	}

	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = withRestoreShopifyToken(os.Environ(), token)

	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return 0, fmt.Errorf("shopify theme push: %w (stderr: %s)", err, stderr)
	}

	out = trimToJSONStart(out)

	var result ThemePushResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("parse push result: %w (raw: %s)", err, string(out))
	}
	if result.Theme.ID == 0 {
		return 0, fmt.Errorf("push result missing theme id (raw: %s)", string(out))
	}
	return result.Theme.ID, nil
}

// withRestoreShopifyToken adds Shopify CLI token env vars without exposing the
// token via argv. Both documented env vars are set for CLI version compatibility.
func withRestoreShopifyToken(env []string, token string) []string {
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

// trimToJSONStart skips any banner text the CLI prints before JSON output.
func trimToJSONStart(b []byte) []byte {
	for i, c := range b {
		if c == '{' || c == '[' {
			return b[i:]
		}
	}
	return b
}
