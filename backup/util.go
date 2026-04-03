package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

// writeData writes data to a file
func writeData(path string, data []byte) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// downloadFile downloads a file from a URL to a local path
func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// downloadProductImages downloads images for products
func downloadProductImages(ctx context.Context, products []map[string]interface{}, imageDir string, concurrency int) error {
	sem := semaphore.NewWeighted(int64(concurrency))
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	for _, product := range products {
		images, ok := product["images"].([]map[string]interface{})
		if !ok {
			continue
		}

		productID, _ := product["id"].(string)
		if productID == "" {
			continue
		}

		// Extract numeric ID from GID
		productID = extractNumericID(productID)
		if productID == "" {
			continue
		}

		for i, img := range images {
			src, _ := img["src"].(string)
			if src == "" {
				continue
			}

			wg.Add(1)

			go func(url, productID string, index int) {
				defer wg.Done()

				if err := sem.Acquire(ctx, 1); err != nil {
					select {
					case errCh <- fmt.Errorf("semaphore acquire failed: %w", err):
					default:
					}
					return
				}
				defer sem.Release(1)

				// Determine file extension from URL
				ext := ".jpg"
				if strings.Contains(strings.ToLower(url), ".png") {
					ext = ".png"
				} else if strings.Contains(strings.ToLower(url), ".webp") {
					ext = ".webp"
				} else if strings.Contains(strings.ToLower(url), ".gif") {
					ext = ".gif"
				}

				// Build file path
				filePath := filepath.Join(imageDir, productID, fmt.Sprintf("%d%s", index, ext))

				// Download with retry
				var lastErr error
				for attempt := 0; attempt < 3; attempt++ {
					if err := downloadFile(url, filePath); err != nil {
						lastErr = err
						time.Sleep(time.Duration(attempt+1) * time.Second)
						continue
					}
					lastErr = nil
					break
				}

				if lastErr != nil {
					select {
					case errCh <- fmt.Errorf("failed to download %s: %w", url, lastErr):
					default:
					}
				}
			}(src, productID, i)
		}
	}

	// Wait for all downloads to complete
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	// Wait for completion or first error
	select {
	case <-doneCh:
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// extractNumericID extracts numeric ID from Shopify GID
// Example: "gid://shopify/Product/123456789" -> "123456789"
func extractNumericID(gid string) string {
	parts := strings.Split(gid, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}