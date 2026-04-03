package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/btafoya/goshopify-backup/shopify"
)

// REST API types for content backup
type Page struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	BodyHTML       string `json:"body_html"`
	Handle         string `json:"handle"`
	Author         string `json:"author"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	TemplateSuffix string `json:"template_suffix"`
	PublishedAt    string `json:"published_at"`
	ShopifyThemeID int64  `json:"shopify_theme_id"`
}

type Blog struct {
	ID                int64  `json:"id"`
	Title             string `json:"title"`
	Handle            string `json:"handle"`
	AdminGraphqlAPIID string `json:"admin_graphql_api_id"`
	Commentable       string `json:"commentable"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	Tags              string `json:"tags"`
	TemplateSuffix    string `json:"template_suffix"`
}

type Article struct {
	ID                int64  `json:"id"`
	Title             string `json:"title"`
	Author            string `json:"author"`
	BodyHTML          string `json:"body_html"`
	Handle            string `json:"handle"`
	BlogID            int64  `json:"blog_id"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	PublishedAt       string `json:"published_at"`
	TemplateSuffix    string `json:"template_suffix"`
	AdminGraphqlAPIID string `json:"admin_graphql_api_id"`
	UserID            int64  `json:"user_id"`
	SummaryHTML       string `json:"summary_html"`
	Tags              string `json:"tags"`
}

type ShopMetafield struct {
	ID            int64  `json:"id"`
	Namespace     string `json:"namespace"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Type          string `json:"type"`
	OwnerID       int64  `json:"owner_id"`
	OwnerResource string `json:"owner_resource"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// ContentModule backs up pages, blogs, articles, and shop metafields using REST API

type ContentModule struct {
	name string
}

// NewContentModule creates a new content backup module
func NewContentModule() *ContentModule {
	return &ContentModule{
		name: "content",
	}
}

// Name returns the module name
func (m *ContentModule) Name() string {
	return m.name
}

// Run executes the content backup
func (m *ContentModule) Run(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	totalCount := 0
	totalSize := int64(0)

	// Backup pages
	pagesCount, pagesSize, err := m.backupPages(ctx, restClient, outputDir)
	if err != nil {
		return totalCount, totalSize, fmt.Errorf("failed to backup pages: %w", err)
	}
	totalCount += pagesCount
	totalSize += pagesSize

	// Backup blogs
	blogsCount, blogsSize, err := m.backupBlogs(ctx, restClient, outputDir)
	if err != nil {
		return totalCount, totalSize, fmt.Errorf("failed to backup blogs: %w", err)
	}
	totalCount += blogsCount
	totalSize += blogsSize

	// Backup shop metafields
	metafieldsCount, metafieldsSize, err := m.backupShopMetafields(ctx, restClient, outputDir)
	if err != nil {
		return totalCount, totalSize, fmt.Errorf("failed to backup shop metafields: %w", err)
	}
	totalCount += metafieldsCount
	totalSize += metafieldsSize

	return totalCount, totalSize, nil
}

// backupPages fetches all pages via REST API
func (m *ContentModule) backupPages(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	var pagesResponse struct {
		Pages []Page `json:"pages"`
	}
	count, err := restClient.GetPages(ctx, "/pages.json", &pagesResponse)
	if err != nil {
		return 0, 0, err
	}

	// Write to JSON file
	jsonData, err := json.MarshalIndent(pagesResponse.Pages, "", "  ")
	if err != nil {
		return count, 0, fmt.Errorf("failed to marshal pages: %w", err)
	}

	outputPath := outputDir + "/pages.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return count, 0, fmt.Errorf("failed to write pages.json: %w", err)
	}

	return count, int64(len(jsonData)), nil
}

// backupBlogs fetches all blogs and their articles via REST API
func (m *ContentModule) backupBlogs(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	// Fetch blogs
	var blogsResponse struct {
		Blogs []Blog `json:"blogs"`
	}
	_, err := restClient.GetPages(ctx, "/blogs.json", &blogsResponse)
	if err != nil {
		return 0, 0, err
	}

	// Fetch articles for each blog
	articles := make([]Article, 0)
	for _, blog := range blogsResponse.Blogs {
		var articlesResponse struct {
			Articles []Article `json:"articles"`
		}
		_, err := restClient.GetPages(ctx, fmt.Sprintf("/blogs/%d/articles.json", blog.ID), &articlesResponse)
		if err != nil {
			// Log but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch articles for blog %d: %v\n", blog.ID, err)
			continue
		}
		articles = append(articles, articlesResponse.Articles...)
	}

	// Combine blogs and articles
	blogData := map[string]interface{}{
		"blogs":    blogsResponse.Blogs,
		"articles": articles,
	}

	jsonData, err := json.MarshalIndent(blogData, "", "  ")
	if err != nil {
		return len(blogsResponse.Blogs) + len(articles), 0, fmt.Errorf("failed to marshal blogs: %w", err)
	}

	outputPath := outputDir + "/blogs.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return len(blogsResponse.Blogs) + len(articles), 0, fmt.Errorf("failed to write blogs.json: %w", err)
	}

	return len(blogsResponse.Blogs) + len(articles), int64(len(jsonData)), nil
}

// backupShopMetafields fetches all shop metafields via REST API
func (m *ContentModule) backupShopMetafields(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	var metafieldsResponse struct {
		Metafields []ShopMetafield `json:"metafields"`
	}
	count, err := restClient.GetPages(ctx, "/metafields.json", &metafieldsResponse)
	if err != nil {
		return 0, 0, err
	}

	// Write to JSON file
	jsonData, err := json.MarshalIndent(metafieldsResponse.Metafields, "", "  ")
	if err != nil {
		return count, 0, fmt.Errorf("failed to marshal metafields: %w", err)
	}

	outputPath := outputDir + "/metafields.json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return count, 0, fmt.Errorf("failed to write metafields.json: %w", err)
	}

	return count, int64(len(jsonData)), nil
}

// RESTFallbackModule provides REST API fallback for bulk operations
type RESTFallbackModule struct {
	name     string
	entity   string
	endpoint string
}

// NewRESTFallbackModule creates a new REST fallback module
func NewRESTFallbackModule(name, entity, endpoint string) *RESTFallbackModule {
	return &RESTFallbackModule{
		name:     name,
		entity:   entity,
		endpoint: endpoint,
	}
}

// Name returns the module name
func (m *RESTFallbackModule) Name() string {
	return m.name
}

// Run executes the backup using REST API as fallback
func (m *RESTFallbackModule) Run(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error) {
	var results []map[string]interface{}
	count, err := restClient.GetPages(ctx, m.endpoint, &results)
	if err != nil {
		return 0, 0, err
	}

	// Write to JSON file
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return count, 0, fmt.Errorf("failed to marshal %s: %w", m.entity, err)
	}

	outputPath := outputDir + "/" + m.entity + ".json"
	if err := writeFile(outputPath, jsonData); err != nil {
		return count, 0, fmt.Errorf("failed to write %s.json: %w", m.entity, err)
	}

	return count, int64(len(jsonData)), nil
}

// RunWithRetry runs a bulk operation with REST fallback on access denied
func RunWithRetry(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, module BulkModule, restFallback *RESTFallbackModule, outputDir string) (int, int64, string, error) {
	count, size, err := module.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		// Check if it's an access denied error
		if strings.Contains(err.Error(), "ACCESS_DENIED") {
			// Try REST fallback
			fmt.Fprintf(os.Stderr, "Bulk operation access denied, falling back to REST for %s\n", module.Name())
			count, size, err = restFallback.Run(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", fmt.Errorf("REST fallback also failed: %w", err)
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// BulkModule interface for modules that use bulk operations
type BulkModule interface {
	Name() string
	Run(ctx context.Context, graphqlClient *shopify.GraphQLClient, outputDir string) (int, int64, error)
}

// RESTModule interface for modules that use REST API
type RESTModule interface {
	Name() string
	Run(ctx context.Context, restClient *shopify.RESTClient, outputDir string) (int, int64, error)
}

// RunAllModules runs all backup modules
func RunAllModules(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, modules []interface{}, outputDir string) (map[string]ModuleResult, error) {
	results := make(map[string]ModuleResult)

	for _, mod := range modules {
		var moduleName string
		var count int
		var size int64
		var fallback string
		var err error

		switch m := mod.(type) {
		case BulkModule:
			moduleName = m.Name()
			// Try REST fallback if available
			var restFallback *RESTFallbackModule
			if _, ok := mod.(*ProductsModule); ok {
				restFallback = NewRESTFallbackModule("products", "products", "/products.json")
			} else if _, ok := mod.(*CustomersModule); ok {
				restFallback = NewRESTFallbackModule("customers", "customers", "/customers.json")
			} else if _, ok := mod.(*OrdersModule); ok {
				restFallback = NewRESTFallbackModule("orders", "orders", "/orders.json")
			} else if _, ok := mod.(*CollectionsModule); ok {
				restFallback = NewRESTFallbackModule("collections", "collections", "/custom_collections.json")
			}

			if restFallback != nil {
				count, size, fallback, err = RunWithRetry(ctx, graphqlClient, restClient, m, restFallback, outputDir)
			} else {
				count, size, err = m.Run(ctx, graphqlClient, outputDir)
			}

		case RESTModule:
			moduleName = m.Name()
			count, size, err = m.Run(ctx, restClient, outputDir)

		default:
			return nil, fmt.Errorf("unknown module type")
		}

		results[moduleName] = ModuleResult{
			Count:    count,
			FileSize: size,
			Fallback: fallback,
			Error:    err,
		}

		if err != nil {
			// Continue with other modules even if one fails
			fmt.Fprintf(os.Stderr, "Module %s failed: %v\n", moduleName, err)
		}
	}

	return results, nil
}

// ModuleResult represents the result of a backup module
type ModuleResult struct {
	Count    int
	FileSize int64
	Fallback string
	Error    error
}
