package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRenderFailed       = errors.New("dashboard render failed")
	ErrWidgetRenderFailed = errors.New("widget render failed")
	ErrCacheExpired       = errors.New("dashboard cache expired")
)

type RenderMode string

const (
	RenderModeInteractive RenderMode = "interactive"
	RenderModeStatic      RenderMode = "static"
	RenderModePDF         RenderMode = "pdf"
	RenderModeImage       RenderMode = "image"
)

type RenderFormat string

const (
	FormatHTML  RenderFormat = "html"
	FormatJSON  RenderFormat = "json"
	FormatPDF   RenderFormat = "pdf"
	FormatPNG   RenderFormat = "png"
	FormatSVG   RenderFormat = "svg"
)

type RenderOptions struct {
	Mode           RenderMode           `json:"mode"`
	Format         RenderFormat         `json:"format"`
	TimeRange      TimeRangeConfig      `json:"time_range"`
	Variables      map[string]interface{} `json:"variables"`
	Theme          string               `json:"theme"`
	Width          int                  `json:"width"`
	Height         int                  `json:"height"`
	Scale          float64              `json:"scale"`
	IncludeRefresh bool                 `json:"include_refresh"`
	ShowControls   bool                 `json:"show_controls"`
	Locale         string               `json:"locale"`
	Timezone       string               `json:"timezone"`
	DryRun         bool                 `json:"dry_run"`
}

type RenderedDashboard struct {
	DashboardID    uuid.UUID              `json:"dashboard_id"`
	Name           string                 `json:"name"`
	RenderedAt     time.Time              `json:"rendered_at"`
	TimeRange      TimeRangeConfig        `json:"time_range"`
	Widgets        []RenderedWidget       `json:"widgets"`
	Variables      map[string]interface{} `json:"variables"`
	Layout         DashboardLayout        `json:"layout"`
	Settings       DashboardSettings      `json:"settings"`
	Metadata       RenderMetadata         `json:"metadata"`
	Theme          string                 `json:"theme"`
	CacheKey       string                 `json:"cache_key"`
	CacheTTL       time.Duration          `json:"cache_ttl"`
}

type RenderedWidget struct {
	WidgetID     uuid.UUID        `json:"widget_id"`
	Type         WidgetType       `json:"type"`
	Title        string           `json:"title"`
	Data         *WidgetData      `json:"data,omitempty"`
	HTML         string           `json:"html,omitempty"`
	Config       WidgetConfig     `json:"config"`
	Display      WidgetDisplay    `json:"display"`
	Position     WidgetLayout     `json:"position"`
	ErrorMessage string           `json:"error_message,omitempty"`
	RenderTime   time.Duration    `json:"render_time"`
	FromCache    bool             `json:"from_cache"`
}

type RenderMetadata struct {
	TotalRenderTime   time.Duration `json:"total_render_time"`
	WidgetsRendered   int           `json:"widgets_rendered"`
	WidgetsFromCache  int           `json:"widgets_from_cache"`
	WidgetsFailed     int           `json:"widgets_failed"`
	TotalDataPoints   int           `json:"total_data_points"`
	QueriesExecuted   int           `json:"queries_executed"`
	PeakMemoryUsage   int64         `json:"peak_memory_usage"`
	CompressionRatio  float64       `json:"compression_ratio"`
	RenderMode        RenderMode    `json:"render_mode"`
	Format            RenderFormat  `json:"format"`
}

type DashboardCache struct {
	cache map[string]*CachedDashboard
	mu    sync.RWMutex
	ttl   time.Duration
}

type CachedDashboard struct {
	Dashboard   *RenderedDashboard `json:"dashboard"`
	CachedAt    time.Time          `json:"cached_at"`
	ExpiresAt   time.Time          `json:"expires_at"`
	AccessCount int                `json:"access_count"`
	Size        int64              `json:"size"`
}

type Renderer struct {
	manager      *Manager
	widgetMgr    *WidgetManager
	cache        *DashboardCache
	dataProvider DataProvider
}

type DataProvider interface {
	QueryData(ctx context.Context, query *WidgetQuery, timeRange TimeRangeConfig) (*WidgetData, error)
}

func NewRenderer(manager *Manager, widgetMgr *WidgetManager, dataProvider DataProvider) *Renderer {
	return &Renderer{
		manager:      manager,
		widgetMgr:    widgetMgr,
		dataProvider: dataProvider,
		cache: &DashboardCache{
			cache: make(map[string]*CachedDashboard),
			ttl:   5 * time.Minute,
		},
	}
}

func (r *Renderer) RenderDashboard(ctx context.Context, dashboardID uuid.UUID, opts *RenderOptions) (*RenderedDashboard, error) {
	startTime := time.Now()

	if opts == nil {
		opts = &RenderOptions{
			Mode:    RenderModeInteractive,
			Format:  FormatJSON,
			TimeRange: TimeRangeConfig{
				Type:     "relative",
				Relative: "last1h",
			},
			Theme: "light",
		}
	}

	cacheKey := r.generateCacheKey(dashboardID, opts)
	if cached := r.getCachedDashboard(cacheKey); cached != nil {
		cached.AccessCount++
		return cached.Dashboard, nil
	}

	dashboard, err := r.manager.GetDashboard(ctx, dashboardID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRenderFailed, err)
	}

	timeRange := r.resolveTimeRange(dashboard.Config.TimeRange, opts.TimeRange)
	variables := r.resolveVariables(dashboard.Config.Variables, opts.Variables)

	widgets, err := r.widgetMgr.GetWidgetsByDashboard(ctx, dashboardID)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get widgets: %v", ErrRenderFailed, err)
	}

	renderedWidgets := make([]RenderedWidget, 0, len(widgets))
	metadata := RenderMetadata{
		RenderMode: opts.Mode,
		Format:     opts.Format,
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	widgetChan := make(chan RenderedWidget, len(widgets))
	semaphore := make(chan struct{}, 10)

	for _, widget := range widgets {
		if !widget.IsEnabled {
			continue
		}

		wg.Add(1)
		go func(w Widget) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			rendered := r.renderWidget(ctx, w, timeRange, variables, opts)
			widgetChan <- rendered
		}(widget)
	}

	go func() {
		wg.Wait()
		close(widgetChan)
	}()

	for rendered := range widgetChan {
		mu.Lock()
		renderedWidgets = append(renderedWidgets, rendered)
		metadata.TotalDataPoints += rendered.Data.Metadata.DataPointsCount
		if rendered.FromCache {
			metadata.WidgetsFromCache++
		}
		if rendered.ErrorMessage != "" {
			metadata.WidgetsFailed++
		} else {
			metadata.WidgetsRendered++
		}
		mu.Unlock()
	}

	metadata.TotalRenderTime = time.Since(startTime)
	metadata.QueriesExecuted = metadata.WidgetsRendered - metadata.WidgetsFromCache

	rendered := &RenderedDashboard{
		DashboardID: dashboard.ID,
		Name:        dashboard.Name,
		RenderedAt:  time.Now(),
		TimeRange:   timeRange,
		Widgets:     renderedWidgets,
		Variables:   variables,
		Layout:      dashboard.Layout,
		Settings:    dashboard.Settings,
		Metadata:    metadata,
		Theme:       opts.Theme,
		CacheKey:    cacheKey,
		CacheTTL:    r.cache.ttl,
	}

	if !opts.DryRun {
		r.cacheDashboard(cacheKey, rendered)
	}

	return rendered, nil
}

func (r *Renderer) renderWidget(ctx context.Context, widget Widget, timeRange TimeRangeConfig, variables map[string]interface{}, opts *RenderOptions) RenderedWidget {
	startTime := time.Now()

	rendered := RenderedWidget{
		WidgetID: widget.ID,
		Type:     widget.Type,
		Title:    widget.Title,
		Config:   widget.Config,
		Display:  widget.Display,
		Position: r.findWidgetLayout(widget.ID, opts),
	}

	data, err := r.dataProvider.QueryData(ctx, &widget.Query, timeRange)
	if err != nil {
		rendered.ErrorMessage = err.Error()
		rendered.RenderTime = time.Since(startTime)
		return rendered
	}

	rendered.Data = data
	rendered.RenderTime = time.Since(startTime)

	return rendered
}

func (r *Renderer) RenderWidget(ctx context.Context, widgetID uuid.UUID, opts *RenderOptions) (*RenderedWidget, error) {
	widget, err := r.widgetMgr.GetWidget(ctx, widgetID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWidgetRenderFailed, err)
	}

	if opts == nil {
		opts = &RenderOptions{
			Mode:    RenderModeInteractive,
			Format:  FormatJSON,
			TimeRange: TimeRangeConfig{Type: "relative", Relative: "last1h"},
		}
	}

	timeRange := opts.TimeRange
	variables := make(map[string]interface{})

	rendered := r.renderWidget(ctx, *widget, timeRange, variables, opts)
	if rendered.ErrorMessage != "" {
		return nil, fmt.Errorf("%w: %s", ErrWidgetRenderFailed, rendered.ErrorMessage)
	}

	return &rendered, nil
}

func (r *Renderer) RenderDashboardSnapshot(ctx context.Context, dashboardID uuid.UUID, opts *RenderOptions) ([]byte, error) {
	rendered, err := r.RenderDashboard(ctx, dashboardID, opts)
	if err != nil {
		return nil, err
	}

	snapshot := struct {
		Dashboard *RenderedDashboard `json:"dashboard"`
		Snapshot  bool               `json:"snapshot"`
		CreatedAt time.Time          `json:"created_at"`
		ExpiresAt time.Time          `json:"expires_at"`
	}{
		Dashboard: rendered,
		Snapshot:  true,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	return data, nil
}

func (r *Renderer) resolveTimeRange(dashboardRange, optsRange TimeRangeConfig) TimeRangeConfig {
	if optsRange.Type != "" {
		return optsRange
	}
	return dashboardRange
}

func (r *Renderer) resolveVariables(dashboardVars []DashboardVariable, optsVars map[string]interface{}) map[string]interface{} {
	variables := make(map[string]interface{})

	for _, v := range dashboardVars {
		if val, ok := optsVars[v.Name]; ok {
			variables[v.Name] = val
		} else {
			variables[v.Name] = v.Default
		}
	}

	return variables
}

func (r *Renderer) generateCacheKey(dashboardID uuid.UUID, opts *RenderOptions) string {
	key := fmt.Sprintf("%s-%s-%v-%s", dashboardID, opts.TimeRange.Relative, opts.Variables, opts.Theme)
	return key
}

func (r *Renderer) getCachedDashboard(key string) *CachedDashboard {
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()

	cached, exists := r.cache.cache[key]
	if !exists {
		return nil
	}

	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached
}

func (r *Renderer) cacheDashboard(key string, dashboard *RenderedDashboard) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()

	cached := &CachedDashboard{
		Dashboard: dashboard,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(r.cache.ttl),
	}

	if data, err := json.Marshal(dashboard); err == nil {
		cached.Size = int64(len(data))
	}

	r.cache.cache[key] = cached
}

func (r *Renderer) ClearCache() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.cache = make(map[string]*CachedDashboard)
}

func (r *Renderer) ClearDashboardCache(dashboardID uuid.UUID) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()

	for key := range r.cache.cache {
		if len(key) > 36 && key[:36] == dashboardID.String() {
			delete(r.cache.cache, key)
		}
	}
}

func (r *Renderer) GetCacheStats() map[string]interface{} {
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()

	totalSize := int64(0)
	totalAccess := 0
	for _, cached := range r.cache.cache {
		totalSize += cached.Size
		totalAccess += cached.AccessCount
	}

	return map[string]interface{}{
		"total_entries":   len(r.cache.cache),
		"total_size":      totalSize,
		"total_access":    totalAccess,
		"cache_ttl":       r.cache.ttl.String(),
	}
}

func (r *Renderer) findWidgetLayout(widgetID uuid.UUID, opts *RenderOptions) WidgetLayout {
	return WidgetLayout{
		WidgetID: widgetID,
		X:        0,
		Y:        0,
		Width:    6,
		Height:   4,
	}
}

func (r *Renderer) SetCacheTTL(ttl time.Duration) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.ttl = ttl
}

func (r *Renderer) PrefetchDashboard(ctx context.Context, dashboardID uuid.UUID, timeRanges []TimeRangeConfig) error {
	for _, timeRange := range timeRanges {
		opts := &RenderOptions{
			Mode:      RenderModeStatic,
			Format:    FormatJSON,
			TimeRange: timeRange,
			DryRun:    false,
		}
		_, err := r.RenderDashboard(ctx, dashboardID, opts)
		if err != nil {
			return fmt.Errorf("prefetch failed for time range %v: %w", timeRange, err)
		}
	}
	return nil
}
