package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// ImageUploader handles image upload to Shopify
type ImageUploader struct {
	client *ShopifyClient
	logger *log.Logger
}

// Image represents an image to upload
type Image struct {
	Source      string `json:"src"`      // URL or local path
	AltText     string `json:"altText"`
	Position    int    `json:"position"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	OriginalID  string `json:"-"`        // Original ID from backup
}

// NewImageUploader creates a new image uploader
func NewImageUploader(client *ShopifyClient) *ImageUploader {
	return &ImageUploader{
		client: client,
		logger: log.New(os.Stderr),
	}
}

// UploadProductImages uploads images for a product
func (u *ImageUploader) UploadProductImages(ctx context.Context, productID string, images []Image) error {
	if u.client.DryRun {
		u.logger.Infof("Dry run - would upload %d images to product %s", len(images), productID)
		return nil
	}

	for i, img := range images {
		img.Position = i + 1

		// Resolve image source
		imageURL, err := u.resolveImageSource(ctx, img.Source)
		if err != nil {
			u.logger.Warnf("Failed to resolve image source %s: %v", img.Source, err)
			continue
		}

		// Stage image via GraphQL
		stagedURL, err := u.stageImageURL(ctx, imageURL)
		if err != nil {
			u.logger.Warnf("Failed to stage image URL %s: %v", imageURL, err)
			continue
		}

		// Create product image
		if err := u.createProductImage(ctx, productID, stagedURL, img); err != nil {
			u.logger.Warnf("Failed to create product image: %v", err)
			continue
		}

		u.logger.Infof("Uploaded image %d/%d for product %s", i+1, len(images), productID)
	}

	return nil
}

// resolveImageSource resolves an image source to a usable URL
func (u *ImageUploader) resolveImageSource(ctx context.Context, source string) (string, error) {
	// Check if it's a local file path
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "./") {
		return u.uploadLocalFile(ctx, source)
	}

	// Check if it's already a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// Verify URL is accessible
		if _, err := u.headURL(ctx, source); err != nil {
			return "", fmt.Errorf("image URL not accessible: %w", err)
		}
		return source, nil
	}

	// Assume it's a relative path from backup directory
	backupDir := u.client.StoreURL // This would need to be passed properly
	fullPath := filepath.Join(backupDir, source)

	if _, err := os.Stat(fullPath); err == nil {
		return u.uploadLocalFile(ctx, fullPath)
	}

	// Try as Shopify CDN URL
	return source, nil
}

// uploadLocalFile uploads a local file to a temporary storage and returns URL
// For this implementation, we'll use Shopify's file upload API
func (u *ImageUploader) uploadLocalFile(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Get file info for content type
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("get file info: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := "image/jpeg"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	}

	// Read file content
	fileData := make([]byte, fileInfo.Size())
	if _, err := file.Read(fileData); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Use Shopify's staged upload API
	query := `
		mutation stagedUploadsCreate($input: [StagedUploadInput!]!) {
			stagedUploadsCreate(input: $input) {
				stagedTargets {
					url
					resourceUrl
					parameters {
						name
						value
					}
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	filename := filepath.Base(filePath)
	variables := map[string]interface{}{
		"input": []map[string]interface{}{
			{
				"resource":   "IMAGE",
				"filename":   filename,
				"mimeType":   contentType,
				"httpMethod": "POST",
				"fileSize":   len(fileData),
			},
		},
	}

	resp, err := u.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create staged upload: %w", err)
	}

	var data struct {
		StagedUploadsCreate struct {
			StagedTargets []struct {
				URL         string `json:"url"`
				ResourceURL string `json:"resourceUrl"`
				Parameters  []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"parameters"`
			} `json:"stagedTargets"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"stagedUploadsCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.StagedUploadsCreate.UserErrors) > 0 {
		return "", fmt.Errorf("staged upload errors: %v", data.StagedUploadsCreate.UserErrors)
	}

	if len(data.StagedUploadsCreate.StagedTargets) == 0 {
		return "", fmt.Errorf("no staged upload targets returned")
	}

	target := data.StagedUploadsCreate.StagedTargets[0]

	// Upload file to the staged URL
	if err := u.uploadToURL(ctx, target.URL, fileData, contentType, target.Parameters); err != nil {
		return "", fmt.Errorf("upload to staged URL: %w", err)
	}

	return target.ResourceURL, nil
}

// stageImageURL stages an external image URL
func (u *ImageUploader) stageImageURL(ctx context.Context, imageURL string) (string, error) {
	// For external URLs, we can use Shopify's staged upload with fileUrl parameter
	query := `
		mutation stagedUploadsCreate($input: [StagedUploadInput!]!) {
			stagedUploadsCreate(input: $input) {
				stagedTargets {
					url
					resourceUrl
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": []map[string]interface{}{
			{
				"resource":   "IMAGE",
				"fileUrl":    imageURL,
				"httpMethod": "PUT",
			},
		},
	}

	resp, err := u.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create staged upload: %w", err)
	}

	var data struct {
		StagedUploadsCreate struct {
			StagedTargets []struct {
				URL         string `json:"url"`
				ResourceURL string `json:"resourceUrl"`
			} `json:"stagedTargets"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"stagedUploadsCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.StagedUploadsCreate.UserErrors) > 0 {
		return "", fmt.Errorf("staged upload errors: %v", data.StagedUploadsCreate.UserErrors)
	}

	if len(data.StagedUploadsCreate.StagedTargets) == 0 {
		return "", fmt.Errorf("no staged upload targets returned")
	}

	return data.StagedUploadsCreate.StagedTargets[0].ResourceURL, nil
}

// createProductImage creates a product image in Shopify
func (u *ImageUploader) createProductImage(ctx context.Context, productID, imageURL string, img Image) error {
	query := `
		mutation productImageCreate($input: ProductImageInput!) {
			productImageCreate(input: $input) {
				image {
					id
					url
					altText
					position
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"productId":  productID,
		"src":        imageURL,
		"altText":    img.AltText,
		"position":   img.Position,
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := u.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return fmt.Errorf("create product image: %w", err)
	}

	var data struct {
		ProductImageCreate struct {
			Image struct {
				ID       string `json:"id"`
				URL      string `json:"url"`
				AltText  string `json:"altText"`
				Position int    `json:"position"`
			} `json:"image"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productImageCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.ProductImageCreate.UserErrors) > 0 {
		return fmt.Errorf("product image creation errors: %v", data.ProductImageCreate.UserErrors)
	}

	return nil
}

// uploadToURL uploads data to a URL
func (u *ImageUploader) uploadToURL(ctx context.Context, url string, data []byte, contentType string, params []struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}) error {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	// Add any parameters as headers
	for _, param := range params {
		req.Header.Set(param.Name, param.Value)
	}

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// headURL checks if a URL is accessible
func (u *ImageUploader) headURL(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}