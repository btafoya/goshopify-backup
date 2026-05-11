package shopify

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/btafoya/goshopify-backup/pkg/auth"
	"github.com/go-resty/resty/v2"
)

const (
	RetryCount     = 3
	RetryBaseDelay = 2 * time.Second
	RetryMaxDelay  = 30 * time.Second
)

// RESTClient wraps resty.Client for Shopify REST API
type RESTClient struct {
	client      *resty.Client
	store       string
	accessToken string
	apiVersion  string
	limiter     *RateLimiter
	auth        *auth.Authenticator
}

// NewRESTClient creates a new Shopify REST client with retry configured
func NewRESTClient(cfg *Config) *RESTClient {
	client := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(RetryCount).
		SetRetryWaitTime(RetryBaseDelay).
		SetRetryMaxWaitTime(RetryMaxDelay).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= 500
		})

	return &RESTClient{
		client:      client,
		store:       cfg.Store,
		accessToken: cfg.AccessToken,
		apiVersion:  cfg.APIVersion,
		limiter:     cfg.Limiter,
		auth:        cfg.Authenticator,
	}
}

// token returns the current access token, refreshing via the Authenticator
// when configured.
func (c *RESTClient) token(ctx context.Context) (string, error) {
	if c.auth != nil {
		return c.auth.EnsureToken(ctx)
	}
	return c.accessToken, nil
}

// baseURL returns the REST API base URL
func (c *RESTClient) baseURL() string {
	return fmt.Sprintf("%s/admin/api/%s", c.store, c.apiVersion)
}

// Get performs a GET request with rate limiting
func (c *RESTClient) Get(ctx context.Context, path string, result interface{}) error {
	c.limiter.Wait()

	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("X-Shopify-Access-Token", token).
		SetHeader("Content-Type", "application/json").
		SetResult(result).
		Get(c.baseURL() + path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

// GetPages fetches all pages using cursor pagination (page_info)
// Returns accumulated results in the result slice
func (c *RESTClient) GetPages(ctx context.Context, path string, result interface{}) (int, error) {
	pagePath := path
	totalCount := 0

	for {
		c.limiter.Wait()

		token, err := c.token(ctx)
		if err != nil {
			return totalCount, fmt.Errorf("auth: %w", err)
		}

		resp, err := c.client.R().
			SetContext(ctx).
			SetHeader("X-Shopify-Access-Token", token).
			SetHeader("Content-Type", "application/json").
			SetResult(result).
			Get(c.baseURL() + pagePath)

		if err != nil {
			return totalCount, fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode() >= 400 {
			return totalCount, fmt.Errorf("request failed with status %d: %s", resp.StatusCode(), resp.String())
		}

		// Count items in this page
		if slice, ok := result.([]interface{}); ok {
			totalCount += len(slice)
		}

		// Check for next page via Link header
		linkHeader := resp.Header().Get("Link")
		if linkHeader == "" {
			break
		}

		nextPageInfo, hasNext := parseLinkHeader(linkHeader)
		if !hasNext {
			break
		}

		// Update path with next page_info
		if strings.Contains(pagePath, "?") {
			pagePath = updatePageInfo(pagePath, nextPageInfo)
		} else {
			pagePath = pagePath + "?page_info=" + nextPageInfo
		}
	}

	return totalCount, nil
}

// parseLinkHeader extracts the next page_info from the Link header
func parseLinkHeader(header string) (string, bool) {
	links := strings.Split(header, ",")
	for _, link := range links {
		if strings.Contains(link, `rel="next"`) {
			start := strings.Index(link, "page_info=")
			if start == -1 {
				continue
			}
			start += len("page_info=")
			end := strings.Index(link[start:], "&")
			if end == -1 {
				end = strings.Index(link[start:], ">")
			}
			if end != -1 {
				return link[start : start+end], true
			}
			return link[start:], true
		}
	}
	return "", false
}

// updatePageInfo updates or adds page_info parameter in a URL path
func updatePageInfo(path string, page_info string) string {
	re := regexp.MustCompile(`page_info=[^&]*`)
	if re.MatchString(path) {
		return re.ReplaceAllString(path, "page_info="+page_info)
	}
	return path + "&page_info=" + page_info
}
