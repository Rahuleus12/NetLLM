package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DownloadStatus represents the status of a download
type DownloadStatus string

const (
	DownloadPending   DownloadStatus = "pending"
	DownloadRunning   DownloadStatus = "running"
	DownloadPaused    DownloadStatus = "paused"
	DownloadCompleted DownloadStatus = "completed"
	DownloadFailed    DownloadStatus = "failed"
	DownloadCancelled DownloadStatus = "cancelled"
)

// DownloadProgress represents the progress of a model download
type DownloadProgress struct {
	ModelID          string         `json:"model_id"`
	Status           DownloadStatus `json:"status"`
	Percentage       float64        `json:"percentage"`
	BytesDownloaded  int64          `json:"bytes_downloaded"`
	TotalBytes       int64          `json:"total_bytes"`
	SpeedMbps        float64        `json:"speed_mbps"`
	ETARemaining     int            `json:"eta_seconds"`
	SpeedBytesPerSec float64        `json:"speed_bytes_per_sec"`
	StartedAt        time.Time      `json:"started_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	Error            string         `json:"error,omitempty"`
	ChunkSize        int64          `json:"chunk_size"`
	Threads          int            `json:"threads"`
	Resumable        bool           `json:"resumable"`
}

// DownloadConfig represents download configuration
type DownloadConfig struct {
	MaxThreads       int           `json:"max_threads"`
	ChunkSize        int64         `json:"chunk_size"`
	Timeout          time.Duration `json:"timeout"`
	RetryAttempts    int           `json:"retry_attempts"`
	RetryDelay       time.Duration `json:"retry_delay"`
	MaxSpeed         int64         `json:"max_speed"` // bytes per second, 0 = unlimited
	TempDir          string        `json:"temp_dir"`
	ResumeEnabled    bool          `json:"resume_enabled"`
	ProgressInterval time.Duration `json:"progress_interval"`
}

// DownloadRequest represents a download request
type DownloadRequest struct {
	ModelID      string      `json:"model_id"`
	Source       ModelSource `json:"source"`
	DestPath     string      `json:"dest_path"`
	ExpectedSize int64       `json:"expected_size,omitempty"`
	Checksum     string      `json:"checksum,omitempty"`
	Config       DownloadConfig `json:"config"`
}

// DownloadManager manages model downloads
type DownloadManager struct {
	downloads    map[string]*DownloadProgress
	queues       map[int][]*DownloadRequest // priority -> requests
	active       map[string]context.CancelFunc
	config       DownloadConfig
	registry     ModelRegistry
	mu           sync.RWMutex
	progressChan chan *DownloadProgress
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(registry ModelRegistry, config DownloadConfig) *DownloadManager {
	// Set defaults
	if config.MaxThreads == 0 {
		config.MaxThreads = 4
	}
	if config.ChunkSize == 0 {
		config.ChunkSize = 10 * 1024 * 1024 // 10MB chunks
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}
	if config.ProgressInterval == 0 {
		config.ProgressInterval = 1 * time.Second
	}
	if config.TempDir == "" {
		config.TempDir = os.TempDir()
	}

	return &DownloadManager{
		downloads:    make(map[string]*DownloadProgress),
		queues:       make(map[int][]*DownloadRequest),
		active:       make(map[string]context.CancelFunc),
		config:       config,
		registry:     registry,
		progressChan: make(chan *DownloadProgress, 100),
	}
}

// StartDownload initiates a model download
func (dm *DownloadManager) StartDownload(ctx context.Context, req *DownloadRequest) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Check if download already exists
	if _, exists := dm.downloads[req.ModelID]; exists {
		return fmt.Errorf("download already exists for model %s", req.ModelID)
	}

	// Initialize progress tracking
	progress := &DownloadProgress{
		ModelID:   req.ModelID,
		Status:    DownloadPending,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
		ChunkSize: req.Config.ChunkSize,
		Threads:   req.Config.MaxThreads,
		Resumable: req.Config.ResumeEnabled,
	}

	dm.downloads[req.ModelID] = progress

	// Create cancellable context
	dlCtx, cancel := context.WithCancel(ctx)
	dm.active[req.ModelID] = cancel

	// Start download in goroutine
	go dm.downloadModel(dlCtx, req, progress)

	log.Printf("Download started for model %s", req.ModelID)
	return nil
}

// downloadModel performs the actual download
func (dm *DownloadManager) downloadModel(ctx context.Context, req *DownloadRequest, progress *DownloadProgress) {
	// Update status to running
	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.Status = DownloadRunning
	})

	var err error
	var retryCount int

	// Retry loop
	for retryCount < req.Config.RetryAttempts {
		select {
		case <-ctx.Done():
			dm.updateProgress(progress, func(p *DownloadProgress) {
				p.Status = DownloadCancelled
				p.Error = "Download cancelled"
			})
			return
		default:
		}

		err = dm.performDownload(ctx, req, progress)
		if err == nil {
			// Download successful
			now := time.Now()
			dm.updateProgress(progress, func(p *DownloadProgress) {
				p.Status = DownloadCompleted
				p.Percentage = 100.0
				p.CompletedAt = &now
			})
			log.Printf("Download completed for model %s", req.ModelID)
			return
		}

		retryCount++
		if retryCount < req.Config.RetryAttempts {
			log.Printf("Download failed for model %s (attempt %d/%d): %v. Retrying in %v...",
				req.ModelID, retryCount, req.Config.RetryAttempts, err, req.Config.RetryDelay)

			dm.updateProgress(progress, func(p *DownloadProgress) {
				p.Error = fmt.Sprintf("Attempt %d failed: %v", retryCount, err)
			})

			select {
			case <-ctx.Done():
				dm.updateProgress(progress, func(p *DownloadProgress) {
					p.Status = DownloadCancelled
					p.Error = "Download cancelled"
				})
				return
			case <-time.After(req.Config.RetryDelay):
				// Continue to next retry
			}
		}
	}

	// All retries failed
	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.Status = DownloadFailed
		p.Error = fmt.Sprintf("Download failed after %d attempts: %v", retryCount, err)
	})
	log.Printf("Download failed for model %s: %v", req.ModelID, err)
}

// performDownload performs a single download attempt
func (dm *DownloadManager) performDownload(ctx context.Context, req *DownloadRequest, progress *DownloadProgress) error {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: req.Config.Timeout,
	}

	// Create the request
	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.Source.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if provided
	if req.Source.Username != "" {
		httpReq.SetBasicAuth(req.Source.Username, req.Source.Password)
	}

	// Get file info first (HEAD request to get size)
	headReq, err := http.NewRequestWithContext(ctx, "HEAD", req.Source.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	if req.Source.Username != "" {
		headReq.SetBasicAuth(req.Source.Username, req.Source.Password)
	}

	headResp, err := client.Do(headReq)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	headResp.Body.Close()

	// Get total size
	totalSize := req.ExpectedSize
	if totalSize == 0 {
		if headResp.ContentLength > 0 {
			totalSize = headResp.ContentLength
		}
	}

	// Check if server supports range requests
	acceptsRanges := headResp.Header.Get("Accept-Ranges") == "bytes"

	// Update progress with total size
	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.TotalBytes = totalSize
		p.Resumable = acceptsRanges && req.Config.ResumeEnabled
	})

	// Create destination directory
	destDir := filepath.Dir(req.DestPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check for partial download
	var startPos int64
	tempPath := req.DestPath + ".tmp"

	if req.Config.ResumeEnabled && acceptsRanges {
		if info, err := os.Stat(tempPath); err == nil {
			startPos = info.Size()
			dm.updateProgress(progress, func(p *DownloadProgress) {
				p.BytesDownloaded = startPos
				if totalSize > 0 {
					p.Percentage = float64(startPos) / float64(totalSize) * 100
				}
			})
			log.Printf("Resuming download for model %s from byte %d", req.ModelID, startPos)
		}
	}

	// Add range header if resuming
	if startPos > 0 {
		httpReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startPos))
	}

	// Perform the request
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Update total size from response if not set
	if totalSize == 0 && resp.ContentLength > 0 {
		totalSize = resp.ContentLength
		dm.updateProgress(progress, func(p *DownloadProgress) {
			p.TotalBytes = totalSize
		})
	}

	// Open file for writing (append if resuming)
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if startPos > 0 {
		if _, err := file.Seek(startPos, 0); err != nil {
			return fmt.Errorf("failed to seek file: %w", err)
		}
	}

	// Create progress tracker
	tracker := &downloadTracker{
		startTime:      time.Now(),
		lastUpdate:     time.Now(),
		lastBytes:      startPos,
		totalBytes:     totalSize,
		progressChan:   dm.progressChan,
		progress:       progress,
		updateProgress: dm.updateProgress,
		interval:       req.Config.ProgressInterval,
	}

	// Copy with progress tracking
	buffer := make([]byte, 32*1024) // 32KB buffer
	var written int64

	speedLimiter := newSpeedLimiter(req.Config.MaxSpeed)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Limit speed if configured
		speedLimiter.wait()

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			written, err = file.Write(buffer[:n])
			if err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
			tracker.Write(written)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read response: %w", err)
		}
	}

	// Ensure all data is written to disk
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Close file before renaming
	file.Close()

	// Verify checksum if provided
	if req.Checksum != "" {
		if err := dm.verifyChecksum(tempPath, req.Checksum); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Move temp file to final destination
	if err := os.Rename(tempPath, req.DestPath); err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	return nil
}

// verifyChecksum verifies the checksum of a downloaded file
func (dm *DownloadManager) verifyChecksum(filePath, expectedChecksum string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))

	// Remove sha256: prefix if present
	expectedChecksum = strings.TrimPrefix(expectedChecksum, "sha256:")

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// PauseDownload pauses an active download
func (dm *DownloadManager) PauseDownload(ctx context.Context, modelID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	progress, exists := dm.downloads[modelID]
	if !exists {
		return fmt.Errorf("download not found for model %s", modelID)
	}

	if progress.Status != DownloadRunning {
		return fmt.Errorf("download is not running")
	}

	// Cancel the download
	if cancel, exists := dm.active[modelID]; exists {
		cancel()
		delete(dm.active, modelID)
	}

	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.Status = DownloadPaused
	})

	log.Printf("Download paused for model %s", modelID)
	return nil
}

// ResumeDownload resumes a paused download
func (dm *DownloadManager) ResumeDownload(ctx context.Context, modelID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	progress, exists := dm.downloads[modelID]
	if !exists {
		return fmt.Errorf("download not found for model %s", modelID)
	}

	if progress.Status != DownloadPaused {
		return fmt.Errorf("download is not paused")
	}

	// Get model from registry to recreate request
	model, err := dm.registry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	// Recreate download request
	req := &DownloadRequest{
		ModelID:      modelID,
		Source:       model.Source,
		DestPath:     model.FileInfo.Path,
		ExpectedSize: model.FileInfo.SizeBytes,
		Checksum:     model.Source.Checksum,
		Config:       dm.config,
	}

	// Create new context
	dlCtx, cancel := context.WithCancel(ctx)
	dm.active[modelID] = cancel

	// Update status
	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.Status = DownloadRunning
		p.Error = ""
	})

	// Restart download
	go dm.downloadModel(dlCtx, req, progress)

	log.Printf("Download resumed for model %s", modelID)
	return nil
}

// CancelDownload cancels a download
func (dm *DownloadManager) CancelDownload(ctx context.Context, modelID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	progress, exists := dm.downloads[modelID]
	if !exists {
		return fmt.Errorf("download not found for model %s", modelID)
	}

	// Cancel the download
	if cancel, exists := dm.active[modelID]; exists {
		cancel()
		delete(dm.active, modelID)
	}

	// Remove temp file
	model, err := dm.registry.Get(ctx, modelID)
	if err == nil && model.FileInfo.Path != "" {
		tempPath := model.FileInfo.Path + ".tmp"
		os.Remove(tempPath)
	}

	// Update status
	dm.updateProgress(progress, func(p *DownloadProgress) {
		p.Status = DownloadCancelled
	})

	log.Printf("Download cancelled for model %s", modelID)
	return nil
}

// GetProgress returns the download progress for a model
func (dm *DownloadManager) GetProgress(ctx context.Context, modelID string) (*DownloadProgress, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	progress, exists := dm.downloads[modelID]
	if !exists {
		return nil, fmt.Errorf("download not found for model %s", modelID)
	}

	// Return a copy to avoid race conditions
	copy := *progress
	return &copy, nil
}

// GetAllProgress returns progress for all downloads
func (dm *DownloadManager) GetAllProgress(ctx context.Context) []*DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	progress := make([]*DownloadProgress, 0, len(dm.downloads))
	for _, p := range dm.downloads {
		copy := *p
		progress = append(progress, &copy)
	}

	return progress
}

// StreamProgress streams progress updates for a specific download
func (dm *DownloadManager) StreamProgress(ctx context.Context, modelID string) <-chan *DownloadProgress {
	ch := make(chan *DownloadProgress, 10)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(dm.config.ProgressInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				progress, err := dm.GetProgress(ctx, modelID)
				if err != nil {
					return
				}

				select {
				case ch <- progress:
				case <-ctx.Done():
					return
				}

				// Stop streaming if download is complete
				if progress.Status == DownloadCompleted ||
				   progress.Status == DownloadFailed ||
				   progress.Status == DownloadCancelled {
					return
				}
			}
		}
	}()

	return ch
}

// updateProgress safely updates download progress
func (dm *DownloadManager) updateProgress(progress *DownloadProgress, update func(*DownloadProgress)) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	update(progress)
	progress.UpdatedAt = time.Now()

	// Send to progress channel (non-blocking)
	select {
	case dm.progressChan <- progress:
	default:
		// Channel full, skip
	}
}

// GetProgressChannel returns the global progress channel
func (dm *DownloadManager) GetProgressChannel() <-chan *DownloadProgress {
	return dm.progressChan
}

// downloadTracker tracks download progress
type downloadTracker struct {
	startTime      time.Time
	lastUpdate     time.Time
	lastBytes      int64
	totalBytes     int64
	bytesWritten   int64
	progressChan   chan *DownloadProgress
	progress       *DownloadProgress
	updateProgress func(*DownloadProgress, func(*DownloadProgress))
	interval       time.Duration
	mu             sync.Mutex
}

func (dt *downloadTracker) Write(p []byte) (int, error) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	n := len(p)
	dt.bytesWritten += int64(n)

	now := time.Now()
	if now.Sub(dt.lastUpdate) >= dt.interval {
		// Calculate speed
		elapsed := now.Sub(dt.lastUpdate).Seconds()
		bytesDiff := dt.bytesWritten - dt.lastBytes
		speed := float64(bytesDiff) / elapsed

		// Calculate ETA
		var eta int
		if speed > 0 && dt.totalBytes > 0 {
			remaining := dt.totalBytes - dt.bytesWritten
			eta = int(float64(remaining) / speed)
		}

		// Calculate percentage
		var percentage float64
		if dt.totalBytes > 0 {
			percentage = float64(dt.bytesWritten) / float64(dt.totalBytes) * 100
		}

		// Update progress
		dt.updateProgress(dt.progress, func(p *DownloadProgress) {
			p.BytesDownloaded = dt.bytesWritten
			p.Percentage = percentage
			p.SpeedBytesPerSec = speed
			p.SpeedMbps = speed / 1024 / 1024 * 8 // Convert to Mbps
			p.ETARemaining = eta
		})

		dt.lastUpdate = now
		dt.lastBytes = dt.bytesWritten
	}

	return n, nil
}

// speedLimiter limits download speed
type speedLimiter struct {
	maxSpeed   int64 // bytes per second
	lastCheck  time.Time
	bytesRead  int64
	bucket     int64
	bucketSize int64
}

func newSpeedLimiter(maxSpeed int64) *speedLimiter {
	if maxSpeed == 0 {
		return &speedLimiter{maxSpeed: 0}
	}

	return &speedLimiter{
		maxSpeed:   maxSpeed,
		lastCheck:  time.Now(),
		bucketSize: maxSpeed,
		bucket:     maxSpeed,
	}
}

func (sl *speedLimiter) wait() {
	if sl.maxSpeed == 0 {
		return
	}

	now := time.Now()
	elapsed := now.Sub(sl.lastCheck).Seconds()

	// Add tokens to bucket
	sl.bucket += int64(elapsed * float64(sl.maxSpeed))
	if sl.bucket > sl.bucketSize {
		sl.bucket = sl.bucketSize
	}

	// If bucket is empty, wait
	if sl.bucket <= 0 {
		waitTime := time.Duration(float64(1) / float64(sl.maxSpeed) * float64(time.Second))
		time.Sleep(waitTime)
		sl.bucket = 1
	}

	sl.bucket--
	sl.lastCheck = now
}

// GetQueue returns the download queue
func (dm *DownloadManager) GetQueue(ctx context.Context) []*DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	queue := make([]*DownloadProgress, 0)
	for _, progress := range dm.downloads {
		if progress.Status == DownloadPending || progress.Status == DownloadPaused {
			copy := *progress
			queue = append(queue, &copy)
		}
	}

	return queue
}

// Prioritize sets the priority of a download in the queue
func (dm *DownloadManager) Prioritize(ctx context.Context, modelID string, priority int) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	progress, exists := dm.downloads[modelID]
	if !exists {
		return fmt.Errorf("download not found for model %s", modelID)
	}

	// Priority is implicit based on order in queue
	// For now, we just log it
	log.Printf("Set priority %d for model %s download", priority, modelID)

	dm.updateProgress(progress, func(p *DownloadProgress) {
		// Could add priority field to progress if needed
	})

	return nil
}

// RemoveDownload removes a download record
func (dm *DownloadManager) RemoveDownload(ctx context.Context, modelID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.downloads[modelID]; !exists {
		return fmt.Errorf("download not found for model %s", modelID)
	}

	// Cancel if active
	if cancel, exists := dm.active[modelID]; exists {
		cancel()
		delete(dm.active, modelID)
	}

	delete(dm.downloads, modelID)
	log.Printf("Removed download record for model %s", modelID)

	return nil
}

// GetDownloadStats returns overall download statistics
func (dm *DownloadManager) GetDownloadStats(ctx context.Context) map[string]interface{} {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	stats := map[string]interface{}{
		"total_downloads":     len(dm.downloads),
		"active_downloads":    0,
		"completed_downloads": 0,
		"failed_downloads":    0,
		"paused_downloads":    0,
		"pending_downloads":   0,
		"total_bytes":         int64(0),
		"downloaded_bytes":    int64(0),
	}

	for _, progress := range dm.downloads {
		switch progress.Status {
		case DownloadRunning:
			stats["active_downloads"] = stats["active_downloads"].(int) + 1
		case DownloadCompleted:
			stats["completed_downloads"] = stats["completed_downloads"].(int) + 1
		case DownloadFailed:
			stats["failed_downloads"] = stats["failed_downloads"].(int) + 1
		case DownloadPaused:
			stats["paused_downloads"] = stats["paused_downloads"].(int) + 1
		case DownloadPending:
			stats["pending_downloads"] = stats["pending_downloads"].(int) + 1
		}

		stats["total_bytes"] = stats["total_bytes"].(int64) + progress.TotalBytes
		stats["downloaded_bytes"] = stats["downloaded_bytes"].(int64) + progress.BytesDownloaded
	}

	return stats
}

// ParseContentRange parses a Content-Range header
func ParseContentRange(rangeHeader string) (start, end, total int64, err error) {
	// Example: "bytes 0-999/1000"
	parts := strings.Split(rangeHeader, " ")
	if len(parts) != 2 || parts[0] != "bytes" {
		return 0, 0, 0, fmt.Errorf("invalid content range format")
	}

	rangeParts := strings.Split(parts[1], "/")
	if len(rangeParts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid content range format")
	}

	byteParts := strings.Split(rangeParts[0], "-")
	if len(byteParts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid content range format")
	}

	start, err = strconv.ParseInt(byteParts[0], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid start byte: %w", err)
	}

	end, err = strconv.ParseInt(byteParts[1], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid end byte: %w", err)
	}

	if rangeParts[1] == "*" {
		total = 0
	} else {
		total, err = strconv.ParseInt(rangeParts[1], 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid total bytes: %w", err)
		}
	}

	return start, end, total, nil
}
