package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Marketplace handles plugin marketplace integration
type Marketplace struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	cache      map[string]*MarketplacePlugin
	cacheTTL   time.Duration
}

// NewMarketplace creates a new marketplace client
func NewMarketplace() *Marketplace {
	return &Marketplace{
		baseURL: "https://marketplace.ai-provider.io/api/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:    make(map[string]*MarketplacePlugin),
		cacheTTL: 5 * time.Minute,
	}
}

// SetAPIKey sets the marketplace API key
func (m *Marketplace) SetAPIKey(apiKey string) {
	m.apiKey = apiKey
}

// SetBaseURL sets a custom marketplace URL
func (m *Marketplace) SetBaseURL(baseURL string) {
	m.baseURL = strings.TrimSuffix(baseURL, "/")
}

// Search searches for plugins in the marketplace
func (m *Marketplace) Search(ctx context.Context, filter MarketplaceSearchFilter) ([]MarketplacePlugin, int, error) {
	// Build query parameters
	params := url.Values{}
	if filter.Query != "" {
		params.Set("q", filter.Query)
	}
	if filter.Type != "" {
		params.Set("type", filter.Type)
	}
	if filter.Category != "" {
		params.Set("category", filter.Category)
	}
	if len(filter.Tags) > 0 {
		params.Set("tags", strings.Join(filter.Tags, ","))
	}
	if filter.Verified != nil {
		params.Set("verified", fmt.Sprintf("%t", *filter.Verified))
	}
	if filter.Official != nil {
		params.Set("official", fmt.Sprintf("%t", *filter.Official))
	}
	if filter.SortBy != "" {
		params.Set("sort", filter.SortBy)
	}
	if filter.SortOrder != "" {
		params.Set("order", filter.SortOrder)
	}
	if filter.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", filter.Page))
	}
	if filter.PageSize > 0 {
		params.Set("per_page", fmt.Sprintf("%d", filter.PageSize))
	}

	// Make request
	endpoint := fmt.Sprintf("%s/plugins/search?%s", m.baseURL, params.Encode())

	var response struct {
		Plugins []MarketplacePlugin `json:"plugins"`
		Total   int                 `json:"total"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, 0, fmt.Errorf("search failed: %w", err)
	}

	return response.Plugins, response.Total, nil
}

// GetPlugin gets detailed information about a plugin
func (m *Marketplace) GetPlugin(ctx context.Context, pluginID string) (*MarketplacePlugin, error) {
	// Check cache
	if cached, exists := m.cache[pluginID]; exists {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/plugins/%s", m.baseURL, pluginID)

	var plugin MarketplacePlugin
	if err := m.makeRequest(ctx, "GET", endpoint, nil, &plugin); err != nil {
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}

	// Cache result
	m.cache[pluginID] = &plugin

	return &plugin, nil
}

// GetPluginVersions gets all versions of a plugin
func (m *Marketplace) GetPluginVersions(ctx context.Context, pluginID string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/plugins/%s/versions", m.baseURL, pluginID)

	var response struct {
		Versions []string `json:"versions"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	return response.Versions, nil
}

// DownloadPlugin downloads a plugin from the marketplace
func (m *Marketplace) DownloadPlugin(ctx context.Context, pluginID, version string) (string, error) {
	// Get download URL
	endpoint := fmt.Sprintf("%s/plugins/%s/download", m.baseURL, pluginID)

	params := url.Values{}
	if version != "" {
		params.Set("version", version)
	}

	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set auth header
	if m.apiKey != "" {
		req.Header.Set("X-API-Key", m.apiKey)
	}

	// Make request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("download failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	// Get download URL from response
	var downloadResp struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&downloadResp); err != nil {
		return "", fmt.Errorf("failed to parse download response: %w", err)
	}

	return downloadResp.URL, nil
}

// GetCategories gets all plugin categories
func (m *Marketplace) GetCategories(ctx context.Context) ([]string, error) {
	endpoint := fmt.Sprintf("%s/categories", m.baseURL)

	var response struct {
		Categories []string `json:"categories"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	return response.Categories, nil
}

// GetFeaturedPlugins gets featured plugins from marketplace
func (m *Marketplace) GetFeaturedPlugins(ctx context.Context) ([]MarketplacePlugin, error) {
	endpoint := fmt.Sprintf("%s/plugins/featured", m.baseURL)

	var response struct {
		Plugins []MarketplacePlugin `json:"plugins"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get featured plugins: %w", err)
	}

	return response.Plugins, nil
}

// GetPopularPlugins gets popular plugins from marketplace
func (m *Marketplace) GetPopularPlugins(ctx context.Context, limit int) ([]MarketplacePlugin, error) {
	endpoint := fmt.Sprintf("%s/plugins/popular?limit=%d", m.baseURL, limit)

	var response struct {
		Plugins []MarketplacePlugin `json:"plugins"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get popular plugins: %w", err)
	}

	return response.Plugins, nil
}

// SubmitReview submits a review for a plugin
func (m *Marketplace) SubmitReview(ctx context.Context, pluginID string, rating int, review string) error {
	endpoint := fmt.Sprintf("%s/plugins/%s/reviews", m.baseURL, pluginID)

	payload := map[string]interface{}{
		"rating": rating,
		"review": review,
	}

	if err := m.makeRequest(ctx, "POST", endpoint, payload, nil); err != nil {
		return fmt.Errorf("failed to submit review: %w", err)
	}

	return nil
}

// GetReviews gets reviews for a plugin
func (m *Marketplace) GetReviews(ctx context.Context, pluginID string, page, pageSize int) ([]PluginReview, int, error) {
	endpoint := fmt.Sprintf("%s/plugins/%s/reviews?page=%d&per_page=%d", m.baseURL, pluginID, page, pageSize)

	var response struct {
		Reviews []PluginReview `json:"reviews"`
		Total   int            `json:"total"`
	}

	if err := m.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, 0, fmt.Errorf("failed to get reviews: %w", err)
	}

	return response.Reviews, response.Total, nil
}

// makeRequest makes an HTTP request to the marketplace API
func (m *Marketplace) makeRequest(ctx context.Context, method, endpoint string, payload interface{}, response interface{}) error {
	var req *http.Request
	var err error

	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(payloadBytes)))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
	}

	// Set auth header
	if m.apiKey != "" {
		req.Header.Set("X-API-Key", m.apiKey)
	}

	// Make request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// PluginReview represents a plugin review
type PluginReview struct {
	ID        string    `json:"id"`
	PluginID  string    `json:"plugin_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Rating    int       `json:"rating"`
	Review    string    `json:"review"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Helpful   int       `json:"helpful"`
}

// ClearCache clears the marketplace cache
func (m *Marketplace) ClearCache() {
	m.cache = make(map[string]*MarketplacePlugin)
}
