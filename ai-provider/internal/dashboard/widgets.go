package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrWidgetNotFound      = errors.New("widget not found")
	ErrInvalidWidgetType   = errors.New("invalid widget type")
	ErrInvalidWidgetConfig = errors.New("invalid widget configuration")
)

type WidgetType string

const (
	WidgetTypeLineChart      WidgetType = "line_chart"
	WidgetTypeBarChart       WidgetType = "bar_chart"
	WidgetTypePieChart       WidgetType = "pie_chart"
	WidgetTypeAreaChart      WidgetType = "area_chart"
	WidgetTypeScatterChart   WidgetType = "scatter_chart"
	WidgetTypeGauge          WidgetType = "gauge"
	WidgetTypeStat           WidgetType = "stat"
	WidgetTypeTable          WidgetType = "table"
	WidgetTypeHeatmap        WidgetType = "heatmap"
	WidgetTypeMap            WidgetType = "map"
	WidgetTypeText           WidgetType = "text"
	WidgetTypeAlertList      WidgetType = "alert_list"
	WidgetTypeLogs           WidgetType = "logs"
	WidgetTypeTrace          WidgetType = "trace"
	WidgetTypeNews           WidgetType = "news"
	WidgetTypeDashboardList  WidgetType = "dashboard_list"
)

type Widget struct {
	ID           uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	DashboardID  uuid.UUID      `json:"dashboard_id" gorm:"type:uuid;not null;index"`
	Type         WidgetType     `json:"type" gorm:"not null;index"`
	Title        string         `json:"title" gorm:"not null"`
	Description  string         `json:"description"`
	Config       WidgetConfig   `json:"config" gorm:"type:jsonb"`
	Query        WidgetQuery    `json:"query" gorm:"type:jsonb"`
	Display      WidgetDisplay  `json:"display" gorm:"type:jsonb"`
	Interactions WidgetInteractions `json:"interactions" gorm:"type:jsonb"`
	CreatedAt    time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	Position     int            `json:"position" gorm:"default:0"`
	IsEnabled    bool           `json:"is_enabled" gorm:"default:true"`
}

type WidgetConfig struct {
	Datasource      string                 `json:"datasource"`
	RefreshInterval string                 `json:"refresh_interval"`
	TimeShift       string                 `json:"time_shift"`
	MaxDataPoints   int                    `json:"max_data_points"`
	MinInterval     string                 `json:"min_interval"`
	CacheTimeout    string                 `json:"cache_timeout"`
	Overrides       []ConfigOverride       `json:"overrides"`
	Transformations []Transformation       `json:"transformations"`
	Options         map[string]interface{} `json:"options"`
}

type ConfigOverride struct {
	Matcher     OverrideMatcher `json:"matcher"`
	Properties  []Property      `json:"properties"`
}

type OverrideMatcher struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // "byName", "byRegexp", "byType", "byFrameRefID"
	Options string `json:"options"`
}

type Property struct {
	ID    string      `json:"id"`
	Value interface{} `json:"value"`
}

type Transformation struct {
	ID      string                 `json:"id"`
	Options map[string]interface{} `json:"options"`
	Disabled bool                  `json:"disabled"`
}

type WidgetQuery struct {
	RefID         string        `json:"ref_id"`
	QueryType     string        `json:"query_type"`
	Target        QueryTarget   `json:"target"`
	Format        string        `json:"format"` // "time_series", "table", "logs"
	MaxDataPoints int64         `json:"max_data_points"`
	MinInterval   string        `json:"min_interval"`
	Interval      string        `json:"interval"`
	TimeRange     TimeRangeConfig `json:"time_range"`
	Hide          bool          `json:"hide"`
	QueryText     string        `json:"query_text"`
	RawQuery      string        `json:"raw_query"`
}

type QueryTarget struct {
	Metric      string            `json:"metric"`
	Filters     []QueryFilter     `json:"filters"`
	GroupBy     []string          `json:"group_by"`
	Aggregation string            `json:"aggregation"` // "avg", "sum", "count", "max", "min"
	Alias       string            `json:"alias"`
	Tags        map[string]string `json:"tags"`
}

type QueryFilter struct {
	Key      string      `json:"key"`
	Operator string      `json:"operator"` // "=", "!=", ">", "<", ">=", "<=", "=~", "!~"
	Value    interface{} `json:"value"`
}

type WidgetDisplay struct {
	ShowLegend      bool              `json:"show_legend"`
	LegendPosition  string            `json:"legend_position"` // "bottom", "right", "top"
	LegendValues    []string          `json:"legend_values"` // "min", "max", "avg", "current", "total"
	LineWidth       int               `json:"line_width"`
	FillOpacity     int               `json:"fill_opacity"`
	PointSize       int               `json:"point_size"`
	StackMode       string            `json:"stack_mode"` // "normal", "percent", "none"
	NullPointMode   string            `json:"null_point_mode"` // "connected", "null", "null as zero"
	GradientMode    string            `json:"gradient_mode"` // "none", "opacity", "hue"
	AxisPlacement   string            `json:"axis_placement"` // "auto", "left", "right", "hidden"
	ScaleDistribution ScaleDistribution `json:"scale_distribution"`
	Thresholds      []Threshold       `json:"thresholds"`
	ColorScheme     ColorScheme       `json:"color_scheme"`
	TooltipMode     string            `json:"tooltip_mode"` // "single", "multi", "none"
	TooltipSort     string            `json:"tooltip_sort"` // "asc", "desc", "none"
}

type ScaleDistribution struct {
	Type          string  `json:"type"` // "linear", "log", "symlog"
	Log           float64 `json:"log"`
	LinearStart   float64 `json:"linear_start"`
	LinearWidth   float64 `json:"linear_width"`
}

type Threshold struct {
	Value float64 `json:"value"`
	Color string  `json:"color"`
	State string  `json:"state"` // "ok", "warning", "critical"
}

type ColorScheme struct {
	Mode       string   `json:"mode"` // "palette", "continuous", "thresholds", "scheme"
	Scheme     string   `json:"scheme"` // "Spectral", "Blues", "Greens", etc.
	Reverse    bool     `json:"reverse"`
	ByValue    bool     `json:"by_value"`
	Steps      []string `json:"steps"`
}

type WidgetInteractions struct {
	DrilldownLinks []DrilldownLink `json:"drilldown_links"`
	CrossFilters   []CrossFilter   `json:"cross_filters"`
	URLLinks       []URLLink       `json:"url_links"`
	Variables      []VariableLink  `json:"variables"`
}

type DrilldownLink struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Target      string            `json:"target"` // "dashboard", "external"
	DashboardID uuid.UUID         `json:"dashboard_id,omitempty"`
	URL         string            `json:"url,omitempty"`
	Variables   map[string]string `json:"variables"`
	IncludeTime bool              `json:"include_time"`
	OpenInNewTab bool             `json:"open_in_new_tab"`
}

type CrossFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Enabled  bool   `json:"enabled"`
}

type URLLink struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	OpenInNewTab bool  `json:"open_in_new_tab"`
}

type VariableLink struct {
	VariableName string `json:"variable_name"`
	SourceField  string `json:"source_field"`
}

type CreateWidgetRequest struct {
	DashboardID  uuid.UUID      `json:"dashboard_id" binding:"required"`
	Type         WidgetType     `json:"type" binding:"required"`
	Title        string         `json:"title" binding:"required"`
	Description  string         `json:"description"`
	Config       WidgetConfig   `json:"config"`
	Query        WidgetQuery    `json:"query"`
	Display      WidgetDisplay  `json:"display"`
	Interactions WidgetInteractions `json:"interactions"`
	Position     int            `json:"position"`
	IsEnabled    bool           `json:"is_enabled"`
}

type UpdateWidgetRequest struct {
	Title        *string            `json:"title,omitempty"`
	Description  *string            `json:"description,omitempty"`
	Config       *WidgetConfig      `json:"config,omitempty"`
	Query        *WidgetQuery       `json:"query,omitempty"`
	Display      *WidgetDisplay     `json:"display,omitempty"`
	Interactions *WidgetInteractions `json:"interactions,omitempty"`
	Position     *int               `json:"position,omitempty"`
	IsEnabled    *bool              `json:"is_enabled,omitempty"`
}

type WidgetData struct {
	WidgetID   uuid.UUID        `json:"widget_id"`
	TimeRange  TimeRangeConfig  `json:"time_range"`
	DataPoints []DataPoint      `json:"data_points"`
	Series     []Series         `json:"series"`
	Metadata   WidgetMetadata   `json:"metadata"`
}

type DataPoint struct {
	Timestamp time.Time               `json:"timestamp"`
	Value     float64                 `json:"value"`
	Labels    map[string]string       `json:"labels"`
}

type Series struct {
	Name       string            `json:"name"`
	RefID      string            `json:"ref_id"`
	Points     []DataPoint       `json:"points"`
	Labels     map[string]string `json:"labels"`
	Meta       SeriesMeta        `json:"meta"`
}

type SeriesMeta struct {
	Custom     map[string]interface{} `json:"custom"`
	ExecutedBy string                 `json:"executed_by"`
}

type WidgetMetadata struct {
	ExecutionTime   time.Duration `json:"execution_time"`
	DataPointsCount int           `json:"data_points_count"`
	SeriesCount     int           `json:"series_count"`
	FromCache       bool          `json:"from_cache"`
	CachedAt        *time.Time    `json:"cached_at,omitempty"`
	Query           string        `json:"query"`
	Error           string        `json:"error,omitempty"`
}

type WidgetLibraryItem struct {
	Type        WidgetType     `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Icon        string         `json:"icon"`
	DefaultConfig WidgetConfig `json:"default_config"`
	DefaultQuery WidgetQuery   `json:"default_query"`
	DefaultDisplay WidgetDisplay `json:"default_display"`
}

type WidgetManager struct {
	db *gorm.DB
}

func NewWidgetManager(db *gorm.DB) *WidgetManager {
	return &WidgetManager{db: db}
}

func (wm *WidgetManager) CreateWidget(ctx context.Context, req *CreateWidgetRequest) (*Widget, error) {
	if err := wm.validateWidgetRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWidgetConfig, err)
	}

	widget := &Widget{
		DashboardID:  req.DashboardID,
		Type:         req.Type,
		Title:        req.Title,
		Description:  req.Description,
		Config:       req.Config,
		Query:        req.Query,
		Display:      req.Display,
		Interactions: req.Interactions,
		Position:     req.Position,
		IsEnabled:    req.IsEnabled,
	}

	if widget.Position == 0 {
		widget.Position = wm.getNextWidgetPosition(ctx, req.DashboardID)
	}

	if err := wm.db.WithContext(ctx).Create(widget).Error; err != nil {
		return nil, fmt.Errorf("failed to create widget: %w", err)
	}

	return widget, nil
}

func (wm *WidgetManager) GetWidget(ctx context.Context, id uuid.UUID) (*Widget, error) {
	var widget Widget
	err := wm.db.WithContext(ctx).
		Where("id = ?", id).
		First(&widget).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWidgetNotFound
		}
		return nil, fmt.Errorf("failed to get widget: %w", err)
	}

	return &widget, nil
}

func (wm *WidgetManager) UpdateWidget(ctx context.Context, id uuid.UUID, req *UpdateWidgetRequest) (*Widget, error) {
	widget, err := wm.GetWidget(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Config != nil {
		updates["config"] = *req.Config
	}
	if req.Query != nil {
		updates["query"] = *req.Query
	}
	if req.Display != nil {
		updates["display"] = *req.Display
	}
	if req.Interactions != nil {
		updates["interactions"] = *req.Interactions
	}
	if req.Position != nil {
		updates["position"] = *req.Position
	}
	if req.IsEnabled != nil {
		updates["is_enabled"] = *req.IsEnabled
	}

	if len(updates) > 0 {
		if err := wm.db.WithContext(ctx).Model(widget).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update widget: %w", err)
		}
	}

	return wm.GetWidget(ctx, id)
}

func (wm *WidgetManager) DeleteWidget(ctx context.Context, id uuid.UUID) error {
	result := wm.db.WithContext(ctx).Delete(&Widget{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete widget: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrWidgetNotFound
	}
	return nil
}

func (wm *WidgetManager) GetWidgetsByDashboard(ctx context.Context, dashboardID uuid.UUID) ([]Widget, error) {
	var widgets []Widget
	err := wm.db.WithContext(ctx).
		Where("dashboard_id = ?", dashboardID).
		Order("position ASC").
		Find(&widgets).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get widgets: %w", err)
	}

	return widgets, nil
}

func (wm *WidgetManager) ReorderWidgets(ctx context.Context, dashboardID uuid.UUID, widgetIDs []uuid.UUID) error {
	tx := wm.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for i, widgetID := range widgetIDs {
		if err := tx.Model(&Widget{}).
			Where("id = ? AND dashboard_id = ?", widgetID, dashboardID).
			Update("position", i).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to reorder widgets: %w", err)
		}
	}

	return tx.Commit().Error
}

func (wm *WidgetManager) DuplicateWidget(ctx context.Context, id uuid.UUID, newTitle string) (*Widget, error) {
	original, err := wm.GetWidget(ctx, id)
	if err != nil {
		return nil, err
	}

	duplicate := &Widget{
		DashboardID:  original.DashboardID,
		Type:         original.Type,
		Title:        newTitle,
		Description:  original.Description,
		Config:       original.Config,
		Query:        original.Query,
		Display:      original.Display,
		Interactions: original.Interactions,
		Position:     wm.getNextWidgetPosition(ctx, original.DashboardID),
		IsEnabled:    original.IsEnabled,
	}

	if err := wm.db.WithContext(ctx).Create(duplicate).Error; err != nil {
		return nil, fmt.Errorf("failed to duplicate widget: %w", err)
	}

	return duplicate, nil
}

func (wm *WidgetManager) GetWidgetLibrary() []WidgetLibraryItem {
	return []WidgetLibraryItem{
		{
			Type:        WidgetTypeLineChart,
			Name:        "Line Chart",
			Description: "Display time-series data as lines",
			Category:    "Charts",
			Icon:        "chart-line",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 1000,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend:     true,
				LegendPosition: "bottom",
				LineWidth:      1,
				FillOpacity:    10,
				NullPointMode:  "connected",
			},
		},
		{
			Type:        WidgetTypeBarChart,
			Name:        "Bar Chart",
			Description: "Display data as vertical or horizontal bars",
			Category:    "Charts",
			Icon:        "chart-bar",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 100,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend:     true,
				LegendPosition: "bottom",
				StackMode:      "normal",
			},
		},
		{
			Type:        WidgetTypePieChart,
			Name:        "Pie Chart",
			Description: "Display data as a pie or donut chart",
			Category:    "Charts",
			Icon:        "chart-pie",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 50,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend:     true,
				LegendPosition: "right",
			},
		},
		{
			Type:        WidgetTypeGauge,
			Name:        "Gauge",
			Description: "Display a single value as a gauge",
			Category:    "Stats",
			Icon:        "tachometer-alt",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 1,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend: false,
				Thresholds: []Threshold{
					{Value: 0, Color: "green", State: "ok"},
					{Value: 50, Color: "yellow", State: "warning"},
					{Value: 80, Color: "red", State: "critical"},
				},
			},
		},
		{
			Type:        WidgetTypeStat,
			Name:        "Stat",
			Description: "Display a single large stat value",
			Category:    "Stats",
			Icon:        "hashtag",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 1,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend: false,
				ColorScheme: ColorScheme{
					Mode: "thresholds",
				},
			},
		},
		{
			Type:        WidgetTypeTable,
			Name:        "Table",
			Description: "Display data in a table format",
			Category:    "Data",
			Icon:        "table",
			DefaultDisplay: WidgetDisplay{
				ShowLegend: false,
			},
		},
		{
			Type:        WidgetTypeHeatmap,
			Name:        "Heatmap",
			Description: "Display data as a heatmap",
			Category:    "Charts",
			Icon:        "th",
			DefaultConfig: WidgetConfig{
				MaxDataPoints: 500,
			},
			DefaultDisplay: WidgetDisplay{
				ShowLegend:  true,
				ColorScheme: ColorScheme{Mode: "Spectral"},
			},
		},
		{
			Type:        WidgetTypeText,
			Name:        "Text",
			Description: "Display static text or markdown",
			Category:    "Decorations",
			Icon:        "font",
			DefaultConfig: WidgetConfig{
				Options: map[string]interface{}{
					"mode":  "markdown",
					"content": "# Title\nEnter your text here",
				},
			},
		},
		{
			Type:        WidgetTypeAlertList,
			Name:        "Alert List",
			Description: "Display list of alerts",
			Category:    "Alerting",
			Icon:        "bell",
			DefaultDisplay: WidgetDisplay{
				ShowLegend: false,
			},
		},
		{
			Type:        WidgetTypeLogs,
			Name:        "Logs",
			Description: "Display log entries",
			Category:    "Data",
			Icon:        "align-left",
			DefaultDisplay: WidgetDisplay{
				ShowLegend: false,
			},
		},
	}
}

func (wm *WidgetManager) GetWidgetTypes() []WidgetType {
	return []WidgetType{
		WidgetTypeLineChart,
		WidgetTypeBarChart,
		WidgetTypePieChart,
		WidgetTypeAreaChart,
		WidgetTypeScatterChart,
		WidgetTypeGauge,
		WidgetTypeStat,
		WidgetTypeTable,
		WidgetTypeHeatmap,
		WidgetTypeMap,
		WidgetTypeText,
		WidgetTypeAlertList,
		WidgetTypeLogs,
		WidgetTypeTrace,
		WidgetTypeNews,
		WidgetTypeDashboardList,
	}
}

func (wm *WidgetManager) ExportWidget(ctx context.Context, id uuid.UUID) ([]byte, error) {
	widget, err := wm.GetWidget(ctx, id)
	if err != nil {
		return nil, err
	}

	export := struct {
		Widget
		ExportVersion string    `json:"export_version"`
		ExportedAt    time.Time `json:"exported_at"`
	}{
		Widget:        *widget,
		ExportVersion: "1.0",
		ExportedAt:    time.Now(),
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to export widget: %w", err)
	}

	return data, nil
}

func (wm *WidgetManager) ImportWidget(ctx context.Context, data []byte, dashboardID uuid.UUID) (*Widget, error) {
	var import struct {
		Widget
		ExportVersion string    `json:"export_version"`
		ExportedAt    time.Time `json:"exported_at"`
	}

	if err := json.Unmarshal(data, &import); err != nil {
		return nil, fmt.Errorf("failed to parse widget import: %w", err)
	}

	widget := &Widget{
		DashboardID:  dashboardID,
		Type:         import.Type,
		Title:        import.Title,
		Description:  import.Description,
		Config:       import.Config,
		Query:        import.Query,
		Display:      import.Display,
		Interactions: import.Interactions,
		Position:     wm.getNextWidgetPosition(ctx, dashboardID),
		IsEnabled:    import.IsEnabled,
	}

	if err := wm.db.WithContext(ctx).Create(widget).Error; err != nil {
		return nil, fmt.Errorf("failed to import widget: %w", err)
	}

	return widget, nil
}

func (wm *WidgetManager) validateWidgetRequest(req *CreateWidgetRequest) error {
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}

	if req.DashboardID.IsZero() {
		return fmt.Errorf("dashboard_id is required")
	}

	if !isValidWidgetType(req.Type) {
		return fmt.Errorf("invalid widget type: %s", req.Type)
	}

	return nil
}

func isValidWidgetType(widgetType WidgetType) bool {
	validTypes := map[WidgetType]bool{
		WidgetTypeLineChart:     true,
		WidgetTypeBarChart:      true,
		WidgetTypePieChart:      true,
		WidgetTypeAreaChart:     true,
		WidgetTypeScatterChart:  true,
		WidgetTypeGauge:         true,
		WidgetTypeStat:          true,
		WidgetTypeTable:         true,
		WidgetTypeHeatmap:       true,
		WidgetTypeMap:           true,
		WidgetTypeText:          true,
		WidgetTypeAlertList:     true,
		WidgetTypeLogs:          true,
		WidgetTypeTrace:         true,
		WidgetTypeNews:          true,
		WidgetTypeDashboardList: true,
	}
	return validTypes[widgetType]
}

func (wm *WidgetManager) getNextWidgetPosition(ctx context.Context, dashboardID uuid.UUID) int {
	var maxPosition int
	wm.db.WithContext(ctx).
		Model(&Widget{}).
		Where("dashboard_id = ?", dashboardID).
		Select("COALESCE(MAX(position), -1)").
		Scan(&maxPosition)
	return maxPosition + 1
}

func (wm *WidgetManager) GetWidgetsByType(ctx context.Context, dashboardID uuid.UUID, widgetType WidgetType) ([]Widget, error) {
	var widgets []Widget
	err := wm.db.WithContext(ctx).
		Where("dashboard_id = ? AND type = ?", dashboardID, widgetType).
		Order("position ASC").
		Find(&widgets).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get widgets by type: %w", err)
	}

	return widgets, nil
}

func (wm *WidgetManager) EnableWidget(ctx context.Context, id uuid.UUID) error {
	return wm.db.WithContext(ctx).
		Model(&Widget{}).
		Where("id = ?", id).
		Update("is_enabled", true).Error
}

func (wm *WidgetManager) DisableWidget(ctx context.Context, id uuid.UUID) error {
	return wm.db.WithContext(ctx).
		Model(&Widget{}).
		Where("id = ?", id).
		Update("is_enabled", false).Error
}
