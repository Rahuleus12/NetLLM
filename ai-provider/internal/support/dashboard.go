package support

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DashboardType defines the type of dashboard
type DashboardType string

const (
	DashboardOverview    DashboardType = "overview"
	DashboardSystem      DashboardType = "system"
	DashboardTickets     DashboardType = "tickets"
	DashboardKnowledge   DashboardType = "knowledge"
	DashboardMetrics     DashboardType = "metrics"
	DashboardAlerts      DashboardType = "alerts"
)

// WidgetType defines the type of dashboard widget
type WidgetType string

const (
	WidgetSystemHealth    WidgetType = "system_health"
	WidgetTicketSummary   WidgetType = "ticket_summary"
	WidgetRecentTickets   WidgetType = "recent_tickets"
	WidgetKnowledgeSearch WidgetType = "knowledge_search"
	WidgetMetrics         WidgetType = "metrics"
	WidgetAlerts          WidgetType = "alerts"
	WidgetActivity        WidgetType = "activity"
	WidgetQuickActions    WidgetType = "quick_actions"
)

// Severity represents severity level
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// AlertStatus represents alert status
type AlertStatus string

const (
	AlertActive   AlertStatus = "active"
	AlertResolved AlertStatus = "resolved"
	AlertAcknowledged AlertStatus = "acknowledged"
)

// SystemHealth represents system health information
type SystemHealth struct {
	Component     string        `json:"component"`
	Status        string        `json:"status"`
	HealthScore   int           `json:"health_score"`
	ResponseTime  time.Duration `json:"response_time"`
	LastCheck     time.Time     `json:"last_check"`
	Uptime        time.Duration `json:"uptime"`
	ErrorRate     float64       `json:"error_rate"`
	Throughput    int64         `json:"throughput"`
	ActiveUsers   int           `json:"active_users"`
	CPUUsage      float64       `json:"cpu_usage"`
	MemoryUsage   float64       `json:"memory_usage"`
	DiskUsage     float64       `json:"disk_usage"`
}

// Alert represents a system alert
type Alert struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Message     string       `json:"message"`
	Severity    Severity     `json:"severity"`
	Status      AlertStatus  `json:"status"`
	Component   string       `json:"component"`
	Timestamp   time.Time    `json:"timestamp"`
	AcknowledgedBy string    `json:"acknowledged_by"`
	ResolvedAt  time.Time    `json:"resolved_at"`
	Tags        []string     `json:"tags"`
}

// Activity represents a system activity
type Activity struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	User        string    `json:"user"`
	Timestamp   time.Time `json:"timestamp"`
	ResourceID  string    `json:"resource_id"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Widget represents a dashboard widget
type Widget struct {
	ID          string                 `json:"id"`
	Type        WidgetType             `json:"type"`
	Title       string                 `json:"title"`
	Position    int                    `json:"position"`
	Size        string                 `json:"size"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
	Data        interface{}            `json:"data"`
	LastUpdated time.Time              `json:"last_updated"`
}

// DashboardLayout represents a dashboard layout configuration
type DashboardLayout struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        DashboardType `json:"type"`
	Widgets     []*Widget `json:"widgets"`
	RefreshInterval time.Duration `json:"refresh_interval"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      string    `json:"user_id"`
	IsDefault   bool      `json:"is_default"`
}

// DashboardMetrics represents dashboard metrics
type DashboardMetrics struct {
	TotalTickets      int       `json:"total_tickets"`
	OpenTickets       int       `json:"open_tickets"`
	CriticalTickets   int       `json:"critical_tickets"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	ResolutionRate    float64   `json:"resolution_rate"`
	CustomerSatisfaction float64 `json:"customer_satisfaction"`
	ActiveAlerts      int       `json:"active_alerts"`
	SystemHealthScore int       `json:"system_health_score"`
	KnowledgeArticles int       `json:"knowledge_articles"`
	SearchQueries     int64     `json:"search_queries"`
	LastUpdated       time.Time `json:"last_updated"`
}

// QuickAction represents a quick action item
type QuickAction struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Action      string    `json:"action"`
	Icon        string    `json:"icon"`
	Category    string    `json:"category"`
	Enabled     bool      `json:"enabled"`
	LastUsed    time.Time `json:"last_used"`
}

// SupportDashboard manages the support dashboard
type SupportDashboard struct {
	layouts         map[string]*DashboardLayout
	widgets         map[string]*Widget
	alerts          []*Alert
	activities      []*Activity
	systemHealth    map[string]*SystemHealth
	metrics         *DashboardMetrics
	quickActions    []*QuickAction
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	updateChan      chan *DashboardLayout
}

// NewSupportDashboard creates a new support dashboard
func NewSupportDashboard() *SupportDashboard {
	ctx, cancel := context.WithCancel(context.Background())

	dashboard := &SupportDashboard{
		layouts:      make(map[string]*DashboardLayout),
		widgets:      make(map[string]*Widget),
		alerts:       make([]*Alert, 0),
		activities:   make([]*Activity, 0),
		systemHealth: make(map[string]*SystemHealth),
		quickActions: make([]*QuickAction, 0),
		ctx:          ctx,
		cancel:       cancel,
		updateChan:   make(chan *DashboardLayout, 100),
		metrics: &DashboardMetrics{
			LastUpdated: time.Now(),
		},
	}

	// Initialize default widgets
	dashboard.initializeDefaultWidgets()
	dashboard.initializeDefaultLayout()
	dashboard.initializeQuickActions()

	return dashboard
}

// initializeDefaultWidgets initializes default dashboard widgets
func (sd *SupportDashboard) initializeDefaultWidgets() {
	widgets := []*Widget{
		{
			ID:       "system-health-widget",
			Type:     WidgetSystemHealth,
			Title:    "System Health",
			Position: 1,
			Size:     "large",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 10},
		},
		{
			ID:       "ticket-summary-widget",
			Type:     WidgetTicketSummary,
			Title:    "Ticket Summary",
			Position: 2,
			Size:     "medium",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 30},
		},
		{
			ID:       "recent-tickets-widget",
			Type:     WidgetRecentTickets,
			Title:    "Recent Tickets",
			Position: 3,
			Size:     "medium",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 60, "limit": 10},
		},
		{
			ID:       "alerts-widget",
			Type:     WidgetAlerts,
			Title:    "Active Alerts",
			Position: 4,
			Size:     "medium",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 15},
		},
		{
			ID:       "knowledge-search-widget",
			Type:     WidgetKnowledgeSearch,
			Title:    "Knowledge Search",
			Position: 5,
			Size:     "small",
			Enabled:  true,
			Config:   map[string]interface{}{},
		},
		{
			ID:       "quick-actions-widget",
			Type:     WidgetQuickActions,
			Title:    "Quick Actions",
			Position: 6,
			Size:     "small",
			Enabled:  true,
			Config:   map[string]interface{}{},
		},
		{
			ID:       "metrics-widget",
			Type:     WidgetMetrics,
			Title:    "Key Metrics",
			Position: 7,
			Size:     "medium",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 20},
		},
		{
			ID:       "activity-widget",
			Type:     WidgetActivity,
			Title:    "Recent Activity",
			Position: 8,
			Size:     "medium",
			Enabled:  true,
			Config:   map[string]interface{}{"refresh_interval": 30, "limit": 20},
		},
	}

	for _, widget := range widgets {
		widget.LastUpdated = time.Now()
		sd.widgets[widget.ID] = widget
	}
}

// initializeDefaultLayout initializes the default dashboard layout
func (sd *SupportDashboard) initializeDefaultLayout() {
	layout := &DashboardLayout{
		ID:              "default-layout",
		Name:            "Default Dashboard",
		Type:            DashboardOverview,
		RefreshInterval: 30 * time.Second,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		UserID:         "system",
		IsDefault:      true,
	}

	// Add all widgets to default layout
	for _, widget := range sd.widgets {
		layout.Widgets = append(layout.Widgets, widget)
	}

	sd.layouts[layout.ID] = layout
}

// initializeQuickActions initializes quick actions
func (sd *SupportDashboard) initializeQuickActions() {
	actions := []*QuickAction{
		{
			ID:          "create-ticket",
			Name:        "Create Ticket",
			Description: "Create a new support ticket",
			Action:      "create_ticket",
			Icon:        "ticket",
			Category:    "tickets",
			Enabled:     true,
		},
		{
			ID:          "search-knowledge",
			Name:        "Search Knowledge Base",
			Description: "Search for articles in knowledge base",
			Action:      "search_knowledge",
			Icon:        "search",
			Category:    "knowledge",
			Enabled:     true,
		},
		{
			ID:          "system-status",
			Name:        "System Status",
			Description: "View detailed system status",
			Action:      "view_system_status",
			Icon:        "activity",
			Category:    "system",
			Enabled:     true,
		},
		{
			ID:          "run-diagnostics",
			Name:        "Run Diagnostics",
			Description: "Run system diagnostics",
			Action:      "run_diagnostics",
			Icon:        "tool",
			Category:    "system",
			Enabled:     true,
		},
		{
			ID:          "view-reports",
			Name:        "View Reports",
			Description: "View support reports and analytics",
			Action:      "view_reports",
			Icon:        "bar-chart",
			Category:    "reports",
			Enabled:     true,
		},
		{
			ID:          "escalate-ticket",
			Name:        "Escalate Ticket",
			Description: "Escalate a ticket to higher support level",
			Action:      "escalate_ticket",
			Icon:        "arrow-up",
			Category:    "tickets",
			Enabled:     true,
		},
	}

	sd.quickActions = actions
}

// CreateLayout creates a new dashboard layout
func (sd *SupportDashboard) CreateLayout(layout *DashboardLayout) error {
	if layout == nil {
		return errors.New("layout cannot be nil")
	}

	if layout.ID == "" {
		return errors.New("layout ID is required")
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	if _, exists := sd.layouts[layout.ID]; exists {
		return fmt.Errorf("layout %s already exists", layout.ID)
	}

	layout.CreatedAt = time.Now()
	layout.UpdatedAt = time.Now()

	sd.layouts[layout.ID] = layout

	return nil
}

// UpdateLayout updates an existing dashboard layout
func (sd *SupportDashboard) UpdateLayout(layout *DashboardLayout) error {
	if layout == nil {
		return errors.New("layout cannot be nil")
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	existing, exists := sd.layouts[layout.ID]
	if !exists {
		return fmt.Errorf("layout %s not found", layout.ID)
	}

	layout.CreatedAt = existing.CreatedAt
	layout.UpdatedAt = time.Now()

	sd.layouts[layout.ID] = layout

	select {
	case sd.updateChan <- layout:
	default:
		// Channel full, drop update
	}

	return nil
}

// GetLayout retrieves a dashboard layout by ID
func (sd *SupportDashboard) GetLayout(layoutID string) (*DashboardLayout, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	layout, exists := sd.layouts[layoutID]
	if !exists {
		return nil, fmt.Errorf("layout %s not found", layoutID)
	}

	return layout, nil
}

// GetDefaultLayout returns the default dashboard layout
func (sd *SupportDashboard) GetDefaultLayout() (*DashboardLayout, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	for _, layout := range sd.layouts {
		if layout.IsDefault {
			return layout, nil
		}
	}

	return nil, errors.New("no default layout found")
}

// UpdateWidget updates a widget's data
func (sd *SupportDashboard) UpdateWidget(widgetID string, data interface{}) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	widget, exists := sd.widgets[widgetID]
	if !exists {
		return fmt.Errorf("widget %s not found", widgetID)
	}

	widget.Data = data
	widget.LastUpdated = time.Now()

	return nil
}

// GetWidget retrieves a widget by ID
func (sd *SupportDashboard) GetWidget(widgetID string) (*Widget, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	widget, exists := sd.widgets[widgetID]
	if !exists {
		return nil, fmt.Errorf("widget %s not found", widgetID)
	}

	return widget, nil
}

// UpdateSystemHealth updates system health information
func (sd *SupportDashboard) UpdateSystemHealth(component string, health *SystemHealth) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	health.LastCheck = time.Now()
	sd.systemHealth[component] = health

	return nil
}

// GetSystemHealth retrieves system health for all components
func (sd *SupportDashboard) GetSystemHealth() map[string]*SystemHealth {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	health := make(map[string]*SystemHealth)
	for k, v := range sd.systemHealth {
		health[k] = v
	}

	return health
}

// CreateAlert creates a new alert
func (sd *SupportDashboard) CreateAlert(alert *Alert) error {
	if alert == nil {
		return errors.New("alert cannot be nil")
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	if alert.ID == "" {
		alert.ID = fmt.Sprintf("alert-%d", time.Now().UnixNano())
	}

	alert.Timestamp = time.Now()
	alert.Status = AlertActive

	sd.alerts = append(sd.alerts, alert)

	// Keep only last 1000 alerts
	if len(sd.alerts) > 1000 {
		sd.alerts = sd.alerts[len(sd.alerts)-1000:]
	}

	return nil
}

// AcknowledgeAlert acknowledges an alert
func (sd *SupportDashboard) AcknowledgeAlert(alertID, acknowledgedBy string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, alert := range sd.alerts {
		if alert.ID == alertID {
			alert.Status = AlertAcknowledged
			alert.AcknowledgedBy = acknowledgedBy
			return nil
		}
	}

	return fmt.Errorf("alert %s not found", alertID)
}

// ResolveAlert resolves an alert
func (sd *SupportDashboard) ResolveAlert(alertID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, alert := range sd.alerts {
		if alert.ID == alertID {
			alert.Status = AlertResolved
			alert.ResolvedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("alert %s not found", alertID)
}

// GetActiveAlerts returns all active alerts
func (sd *SupportDashboard) GetActiveAlerts() []*Alert {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	alerts := make([]*Alert, 0)
	for _, alert := range sd.alerts {
		if alert.Status == AlertActive {
			alerts = append(alerts, alert)
		}
	}

	return alerts
}

// AddActivity adds a new activity
func (sd *SupportDashboard) AddActivity(activity *Activity) error {
	if activity == nil {
		return errors.New("activity cannot be nil")
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	if activity.ID == "" {
		activity.ID = fmt.Sprintf("activity-%d", time.Now().UnixNano())
	}

	activity.Timestamp = time.Now()

	sd.activities = append(sd.activities, activity)

	// Keep only last 1000 activities
	if len(sd.activities) > 1000 {
		sd.activities = sd.activities[len(sd.activities)-1000:]
	}

	return nil
}

// GetRecentActivities returns recent activities
func (sd *SupportDashboard) GetRecentActivities(limit int) []*Activity {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	if limit <= 0 || limit > len(sd.activities) {
		limit = len(sd.activities)
	}

	activities := make([]*Activity, limit)
	start := len(sd.activities) - limit
	if start < 0 {
		start = 0
	}
	copy(activities, sd.activities[start:])

	return activities
}

// UpdateMetrics updates dashboard metrics
func (sd *SupportDashboard) UpdateMetrics(metrics *DashboardMetrics) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	metrics.LastUpdated = time.Now()
	sd.metrics = metrics

	return nil
}

// GetMetrics returns current dashboard metrics
func (sd *SupportDashboard) GetMetrics() *DashboardMetrics {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return sd.metrics
}

// GetQuickActions returns all quick actions
func (sd *SupportDashboard) GetQuickActions() []*QuickAction {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	actions := make([]*QuickAction, len(sd.quickActions))
	copy(actions, sd.quickActions)

	return actions
}

// ExecuteQuickAction executes a quick action
func (sd *SupportDashboard) ExecuteQuickAction(actionID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, action := range sd.quickActions {
		if action.ID == actionID {
			if !action.Enabled {
				return fmt.Errorf("action %s is disabled", actionID)
			}
			action.LastUsed = time.Now()
			return nil
		}
	}

	return fmt.Errorf("action %s not found", actionID)
}

// GetDashboardStats returns dashboard statistics
func (sd *SupportDashboard) GetDashboardStats() map[string]interface{} {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	activeAlerts := 0
	criticalAlerts := 0
	for _, alert := range sd.alerts {
		if alert.Status == AlertActive {
			activeAlerts++
			if alert.Severity == SeverityCritical {
				criticalAlerts++
			}
		}
	}

	avgHealthScore := 0
	if len(sd.systemHealth) > 0 {
		totalScore := 0
		for _, health := range sd.systemHealth {
			totalScore += health.HealthScore
		}
		avgHealthScore = totalScore / len(sd.systemHealth)
	}

	return map[string]interface{}{
		"total_layouts":        len(sd.layouts),
		"total_widgets":        len(sd.widgets),
		"active_alerts":        activeAlerts,
		"critical_alerts":      criticalAlerts,
		"total_activities":     len(sd.activities),
		"system_components":    len(sd.systemHealth),
		"avg_health_score":     avgHealthScore,
		"quick_actions":        len(sd.quickActions),
		"last_updated":         sd.metrics.LastUpdated,
	}
}

// Start starts the support dashboard
func (sd *SupportDashboard) Start() error {
	sd.wg.Add(1)
	go sd.refreshWidgets()

	return nil
}

// Stop stops the support dashboard
func (sd *SupportDashboard) Stop() error {
	sd.cancel()
	sd.wg.Wait()
	close(sd.updateChan)
	return nil
}

// refreshWidgets periodically refreshes widget data
func (sd *SupportDashboard) refreshWidgets() {
	defer sd.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sd.ctx.Done():
			return
		case <-ticker.C:
			sd.updateWidgetData()
		case layout := <-sd.updateChan:
			sd.handleLayoutUpdate(layout)
		}
	}
}

// updateWidgetData updates data for all widgets
func (sd *SupportDashboard) updateWidgetData() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Update system health widget
	for _, widget := range sd.widgets {
		if widget.Type == WidgetSystemHealth {
			widget.Data = sd.getSystemHealthData()
			widget.LastUpdated = time.Now()
		} else if widget.Type == WidgetAlerts {
			widget.Data = sd.getActiveAlertsData()
			widget.LastUpdated = time.Now()
		} else if widget.Type == WidgetMetrics {
			widget.Data = sd.metrics
			widget.LastUpdated = time.Now()
		} else if widget.Type == WidgetActivity {
			widget.Data = sd.getRecentActivitiesData(20)
			widget.LastUpdated = time.Now()
		}
	}
}

// getSystemHealthData gets system health data for widget
func (sd *SupportDashboard) getSystemHealthData() map[string]interface{} {
	data := make(map[string]interface{})
	for component, health := range sd.systemHealth {
		data[component] = health
	}
	return data
}

// getActiveAlertsData gets active alerts data for widget
func (sd *SupportDashboard) getActiveAlertsData() []*Alert {
	alerts := make([]*Alert, 0)
	for _, alert := range sd.alerts {
		if alert.Status == AlertActive {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// getRecentActivitiesData gets recent activities data for widget
func (sd *SupportDashboard) getRecentActivitiesData(limit int) []*Activity {
	activities := make([]*Activity, 0)
	start := len(sd.activities) - limit
	if start < 0 {
		start = 0
	}
	for i := len(sd.activities) - 1; i >= start; i-- {
		activities = append(activities, sd.activities[i])
	}
	return activities
}

// handleLayoutUpdate handles layout update events
func (sd *SupportDashboard) handleLayoutUpdate(layout *DashboardLayout) {
	// Could trigger UI updates, notifications, etc.
}
