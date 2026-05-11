package backup

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/btafoya/goshopify-backup/jsonl"
	"github.com/btafoya/goshopify-backup/shopify"
)

// Bulk query definitions
const (
	productsQuery = `{
		products {
			edges {
				node {
					id
					title
					handle
					descriptionHtml
					createdAt
					updatedAt
					vendor
					productType
					tags
					status
					variants(first: 250) {
						edges {
							node {
								id
								title
								price
								sku
								inventoryQuantity
								compareAtPrice
								metafields(first: 50) {
									edges {
										node {
											id
											namespace
											key
											value
											type
										}
									}
								}
							}
						}
					}
					images(first: 250) {
						edges {
							node {
								id
								src
								altText
								width
								height
							}
						}
					}
					metafields(first: 50) {
						edges {
							node {
								id
								namespace
								key
								value
								type
							}
						}
					}
				}
			}
		}
	}`

	customersQuery = `{
		customers {
			edges {
				node {
					id
					email
					firstName
					lastName
					phone
					createdAt
					updatedAt
					state
					addresses {
						id
						address1
						address2
						city
						province
						country
						zip
						phone
					}
					metafields(first: 50) {
						edges {
							node {
								id
								namespace
								key
								value
								type
							}
						}
					}
				}
			}
		}
	}`

	ordersQuery = `{
		orders(first: 250) {
			edges {
				node {
					id
					name
					createdAt
					updatedAt
					processedAt
					displayFinancialStatus
					displayFulfillmentStatus
					totalPrice
					subtotalPrice
					totalTax
					totalDiscounts
					totalShippingPrice
					lineItems(first: 250) {
						edges {
							node {
								id
								title
								quantity
								originalTotalSet {
									presentmentMoney {
										amount
										currencyCode
									}
								}
								sku
								variant {
									id
									title
								}
							}
						}
					}
					fulfillments(first: 250) {
						id
						status
						trackingInfo {
							number
							url
						}
					}
					refunds(first: 250) {
						id
						createdAt
						totalRefundedSet {
							presentmentMoney {
								amount
								currencyCode
							}
						}
					}
					metafields(first: 50) {
						edges {
							node {
								id
								namespace
								key
								value
								type
							}
						}
					}
				}
			}
		}
	}`

	collectionsQuery = `{
		collections(first: 250) {
			edges {
				node {
					id
					title
					handle
					description
					updatedAt
					sortOrder
					products(first: 250) {
						edges {
							node {
								id
							}
						}
					}
					ruleSet {
						rules {
							column
							condition
							relation
						}
					}
					metafields(first: 50) {
						edges {
							node {
								id
								namespace
								key
								value
								type
							}
						}
					}
				}
			}
		}
	}`
)

// ProductsModule backs up products using bulk operations
type ProductsModule struct {
	name             string
	downloadImages   bool
	imageDir         string
	imageConcurrency int
}

// NewProductsModule creates a new products backup module
func NewProductsModule(imageDir string, downloadImages bool, concurrency int) *ProductsModule {
	return &ProductsModule{
		name:             "products",
		imageDir:         imageDir,
		downloadImages:   downloadImages,
		imageConcurrency: concurrency,
	}
}

// Name returns the module name
func (m *ProductsModule) Name() string {
	return m.name
}

// Run executes the products backup
func (m *ProductsModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, productsQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1*time.Second, 10*time.Minute)
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty array
		emptyData := []byte("[]")
		outputPath := outputDir + "/products.json"
		if err := writeFile(outputPath, emptyData); err != nil {
			return 0, 0, fmt.Errorf("failed to write products.json: %w", err)
		}
		return 0, int64(2), nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse and reconstruct JSONL
	parser := jsonl.NewParser(jsonlReader)
	var records []map[string]interface{}
	for parser.Scan() {
		var record map[string]interface{}
		if err := parser.Decode(&record); err != nil {
			continue
		}
		records = append(records, record)
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Reconstruct nested data
	reconstructed, err := jsonl.ReconstructBulkData(records)
	if err != nil {
		return 0, 0, fmt.Errorf("reconstruction failed: %w", err)
	}

	// Write to JSON file
	jsonData, err := jsonl.NormalizeJSONL(reconstructed)
	if err != nil {
		return 0, 0, fmt.Errorf("JSON normalization failed: %w", err)
	}

	outputPath := outputDir + "/products.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return 0, 0, fmt.Errorf("failed to write products.json: %w", err)
	}

	// Download images if enabled
	if m.downloadImages && m.imageDir != "" {
		if err := downloadProductImages(ctx, reconstructed, m.imageDir, m.imageConcurrency); err != nil {
			// Log but don't fail the backup
			fmt.Fprintf(os.Stderr, "Warning: image download failed: %v\n", err)
		}
	}

	return len(reconstructed), int64(len(jsonData)), nil
}

// writeFile writes data to a file (delegates to writeData from util.go)
func writeFile(path string, data []byte) error {
	return writeData(path, data)
}

// isAccessDenied checks if an error is an AccessDeniedError
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	// Check error message for ACCESS_DENIED prefix
	errStr := err.Error()
	return containsString(errStr, "ACCESS_DENIED")
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// findSubstring finds a substring in a string
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CustomersModule backs up customers using bulk operations with REST fallback
type CustomersModule struct {
	name string
}

// NewCustomersModule creates a new customers backup module
func NewCustomersModule() *CustomersModule {
	return &CustomersModule{
		name: "customers",
	}
}

// Name returns the module name
func (m *CustomersModule) Name() string {
	return m.name
}

// Run executes the customers backup
func (m *CustomersModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, customersQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1*time.Second, 10*time.Minute)
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty array
		emptyData := []byte("[]")
		outputPath := outputDir + "/customers.json"
		if err := writeFile(outputPath, emptyData); err != nil {
			return 0, 0, fmt.Errorf("failed to write customers.json: %w", err)
		}
		return 0, int64(2), nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse and reconstruct JSONL
	parser := jsonl.NewParser(jsonlReader)
	var records []map[string]interface{}
	for parser.Scan() {
		var record map[string]interface{}
		if err := parser.Decode(&record); err != nil {
			continue
		}
		records = append(records, record)
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Reconstruct nested data
	reconstructed, err := jsonl.ReconstructBulkData(records)
	if err != nil {
		return 0, 0, fmt.Errorf("reconstruction failed: %w", err)
	}

	// Write to JSON file
	jsonData, err := jsonl.NormalizeJSONL(reconstructed)
	if err != nil {
		return 0, 0, fmt.Errorf("JSON normalization failed: %w", err)
	}

	outputPath := outputDir + "/customers.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return 0, 0, fmt.Errorf("failed to write customers.json: %w", err)
	}

	return len(reconstructed), int64(len(jsonData)), nil
}

// OrdersModule backs up orders using bulk operations with REST fallback
type OrdersModule struct {
	name string
}

// NewOrdersModule creates a new orders backup module
func NewOrdersModule() *OrdersModule {
	return &OrdersModule{
		name: "orders",
	}
}

// Name returns the module name
func (m *OrdersModule) Name() string {
	return m.name
}

// Run executes the orders backup
func (m *OrdersModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, ordersQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1*time.Second, 10*time.Minute)
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty array
		emptyData := []byte("[]")
		outputPath := outputDir + "/orders.json"
		if err := writeFile(outputPath, emptyData); err != nil {
			return 0, 0, fmt.Errorf("failed to write orders.json: %w", err)
		}
		return 0, int64(2), nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse and reconstruct JSONL
	parser := jsonl.NewParser(jsonlReader)
	var records []map[string]interface{}
	for parser.Scan() {
		var record map[string]interface{}
		if err := parser.Decode(&record); err != nil {
			continue
		}
		records = append(records, record)
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Reconstruct nested data
	reconstructed, err := jsonl.ReconstructBulkData(records)
	if err != nil {
		return 0, 0, fmt.Errorf("reconstruction failed: %w", err)
	}

	// Write to JSON file
	jsonData, err := jsonl.NormalizeJSONL(reconstructed)
	if err != nil {
		return 0, 0, fmt.Errorf("JSON normalization failed: %w", err)
	}

	outputPath := outputDir + "/orders.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return 0, 0, fmt.Errorf("failed to write orders.json: %w", err)
	}

	return len(reconstructed), int64(len(jsonData)), nil
}

// CollectionsModule backs up collections using bulk operations
type CollectionsModule struct {
	name string
}

// NewCollectionsModule creates a new collections backup module
func NewCollectionsModule() *CollectionsModule {
	return &CollectionsModule{
		name: "collections",
	}
}

// Name returns the module name
func (m *CollectionsModule) Name() string {
	return m.name
}

// Run executes the collections backup
func (m *CollectionsModule) Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error) {
	// Submit bulk operation
	_, err := graphqlClient.SubmitBulkOperation(ctx, collectionsQuery)
	if err != nil {
		if isAccessDenied(err) {
			return 0, 0, &shopify.AccessDeniedError{Message: "Bulk operation access denied"}
		}
		return 0, 0, fmt.Errorf("failed to submit bulk operation: %w", err)
	}

	// Poll for completion
	downloadURL, err := graphqlClient.PollBulkOperation(ctx, 1*time.Second, 10*time.Minute)
	if err != nil {
		return 0, 0, fmt.Errorf("bulk operation polling failed: %w", err)
	}

	// Check if no data
	if downloadURL == "no-data" {
		// Write empty array
		emptyData := []byte("[]")
		outputPath := outputDir + "/collections.json"
		if err := writeFile(outputPath, emptyData); err != nil {
			return 0, 0, fmt.Errorf("failed to write collections.json: %w", err)
		}
		return 0, int64(2), nil
	}

	// Download JSONL
	jsonlReader, err := graphqlClient.DownloadJSONL(ctx, downloadURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to download JSONL: %w", err)
	}
	defer jsonlReader.Close()

	// Parse and reconstruct JSONL
	parser := jsonl.NewParser(jsonlReader)
	var records []map[string]interface{}
	for parser.Scan() {
		var record map[string]interface{}
		if err := parser.Decode(&record); err != nil {
			continue
		}
		records = append(records, record)
	}

	if err := parser.Err(); err != nil {
		return 0, 0, fmt.Errorf("JSONL parse error: %w", err)
	}

	// Reconstruct nested data
	reconstructed, err := jsonl.ReconstructBulkData(records)
	if err != nil {
		return 0, 0, fmt.Errorf("reconstruction failed: %w", err)
	}

	// Write to JSON file
	jsonData, err := jsonl.NormalizeJSONL(reconstructed)
	if err != nil {
		return 0, 0, fmt.Errorf("JSON normalization failed: %w", err)
	}

	outputPath := outputDir + "/collections.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return 0, 0, fmt.Errorf("failed to write collections.json: %w", err)
	}

	return len(reconstructed), int64(len(jsonData)), nil
}
