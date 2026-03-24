package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// Sandbox provides isolation and security for plugin execution
type Sandbox struct {
	config      *PluginSandboxConfig
	limits      *ResourceLimits
	mu          sync.RWMutex
	activeProcs map[int]*SandboxedProcess
}

// SandboxedProcess represents a process running in the sandbox
type SandboxedProcess struct {
	PID        int
	PluginID   string
	StartTime  time.Time
	Cmd        *exec.Cmd
	CancelFunc context.CancelFunc
	Monitoring bool
}

// DefaultSandboxConfig returns default sandbox configuration
func DefaultSandboxConfig() *PluginSandboxConfig {
	return &PluginSandboxConfig{
		Enabled:          true,
		NetworkAccess:    false,
		FileSystemAccess: []string{},
		EnvironmentVars:  []string{"PATH", "HOME"},
		ResourceLimits: ResourceLimits{
			MaxMemoryMB:     512,
			MaxCPUPercent:   50,
			MaxGoroutines:   100,
			MaxFileSizeMB:   100,
			MaxNetworkConns: 10,
			TimeoutSeconds:  300,
		},
		AllowedSyscalls: []string{
			"read", "write", "open", "close", "stat", "fstat", "lstat",
			"poll", "lseek", "mmap", "mprotect", "munmap", "brk",
			"rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "ioctl",
			"access", "pipe", "select", "sched_yield", "mremap",
			"msync", "mincore", "madvise", "dup", "dup2", "pause",
			"nanosleep", "getitimer", "alarm", "setitimer", "getpid",
			"sendfile", "socket", "connect", "accept", "sendto",
			"recvfrom", "sendmsg", "recvmsg", "shutdown", "bind",
			"listen", "getsockname", "getpeername", "socketpair",
			"setsockopt", "getsockopt", "clone", "fork", "vfork",
			"execve", "exit", "wait4", "kill", "uname",
		},
		SeccompProfile:  "",
		AppArmorProfile: "",
	}
}

// NewSandbox creates a new sandbox instance
func NewSandbox(config *PluginSandboxConfig) (*Sandbox, error) {
	if config == nil {
		config = DefaultSandboxConfig()
	}

	sandbox := &Sandbox{
		config:      config,
		limits:      &config.ResourceLimits,
		activeProcs: make(map[int]*SandboxedProcess),
	}

	return sandbox, nil
}

// ExecuteInSandbox executes a command in the sandbox
func (s *Sandbox) ExecuteInSandbox(ctx context.Context, pluginID string, cmd *exec.Cmd) error {
	if !s.config.Enabled {
		return cmd.Run()
	}

	// Create context with timeout
	timeout := time.Duration(s.limits.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Configure command for sandboxing
	if err := s.configureCommand(cmd); err != nil {
		return fmt.Errorf("failed to configure sandbox: %w", err)
	}

	// Set resource limits
	if err := s.setResourceLimits(cmd); err != nil {
		return fmt.Errorf("failed to set resource limits: %w", err)
	}

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sandboxed process: %w", err)
	}

	// Track process
	sandboxedProc := &SandboxedProcess{
		PID:        cmd.Process.Pid,
		PluginID:   pluginID,
		StartTime:  time.Now(),
		Cmd:        cmd,
		CancelFunc: cancel,
		Monitoring: true,
	}

	s.mu.Lock()
	s.activeProcs[cmd.Process.Pid] = sandboxedProc
	s.mu.Unlock()

	// Start monitoring
	go s.monitorProcess(sandboxedProc)

	// Wait for completion
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		s.removeProcess(cmd.Process.Pid)
		return err
	case <-ctx.Done():
		s.KillProcess(cmd.Process.Pid)
		return fmt.Errorf("process timeout exceeded")
	}
}

// configureCommand configures the command for sandbox execution
func (s *Sandbox) configureCommand(cmd *exec.Cmd) error {
	// Set isolated environment
	env := []string{}
	for _, envVar := range s.config.EnvironmentVars {
		if val := os.Getenv(envVar); val != "" {
			env = append(env, fmt.Sprintf("%s=%s", envVar, val))
		}
	}

	// Add plugin-specific environment variables
	env = append(env, "SANDBOX=1")
	env = append(env, "PLUGIN_MODE=1")
	cmd.Env = env

	// Set working directory to isolated directory
	if len(s.config.FileSystemAccess) > 0 {
		cmd.Dir = s.config.FileSystemAccess[0]
	}

	// Configure process attributes for isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC,
	}

	// Add network namespace if network access is disabled
	if !s.config.NetworkAccess {
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNET
	}

	return nil
}

// setResourceLimits sets resource limits for the command
func (s *Sandbox) setResourceLimits(cmd *exec.Cmd) error {
	// Resource limits will be applied after process starts
	// This is handled in the monitoring goroutine
	return nil
}

// monitorProcess monitors a sandboxed process
func (s *Sandbox) monitorProcess(proc *SandboxedProcess) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !proc.Monitoring {
				return
			}

			// Check if process is still running
			if proc.Cmd.Process == nil {
				return
			}

			// Monitor resource usage
			if err := s.checkResourceUsage(proc); err != nil {
				s.logViolation(proc.PluginID, "resource_limit_exceeded", err.Error())
				s.KillProcess(proc.PID)
				return
			}

		case <-proc.Cmd.Context.Done():
			return
		}
	}
}

// checkResourceUsage checks if process is within resource limits
func (s *Sandbox) checkResourceUsage(proc *SandboxedProcess) error {
	// On Linux, we can read /proc/[pid]/status for memory usage
	// This is a simplified implementation
	if runtime.GOOS == "linux" {
		statusFile := fmt.Sprintf("/proc/%d/status", proc.PID)
		if data, err := os.ReadFile(statusFile); err == nil {
			// Parse VmRSS for memory usage
			// In production, implement proper parsing
			_ = string(data)
		}
	}

	return nil
}

// KillProcess kills a sandboxed process
func (s *Sandbox) KillProcess(pid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	proc, exists := s.activeProcs[pid]
	if !exists {
		return fmt.Errorf("process not found: %d", pid)
	}

	proc.Monitoring = false

	if proc.Cmd.Process != nil {
		if err := proc.Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	delete(s.activeProcs, pid)
	return nil
}

// removeProcess removes a process from tracking
func (s *Sandbox) removeProcess(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeProcs, pid)
}

// IsFileSystemAccessAllowed checks if path access is allowed
func (s *Sandbox) IsFileSystemAccessAllowed(path string) bool {
	if len(s.config.FileSystemAccess) == 0 {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, allowedPath := range s.config.FileSystemAccess {
		absAllowed, err := filepath.Abs(allowedPath)
		if err != nil {
			continue
		}

		if absPath == absAllowed || filepath.HasPrefix(absPath, absAllowed+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// IsNetworkAccessAllowed checks if network access is allowed
func (s *Sandbox) IsNetworkAccessAllowed() bool {
	return s.config.NetworkAccess
}

// CreateIsolatedDirectory creates an isolated directory for plugin
func (s *Sandbox) CreateIsolatedDirectory(pluginID string) (string, error) {
	baseDir := filepath.Join(os.TempDir(), "plugin-sandbox", pluginID)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create isolated directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{"tmp", "data", "logs", "cache"}
	for _, subdir := range subdirs {
		path := filepath.Join(baseDir, subdir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("failed to create subdirectory %s: %w", subdir, err)
		}
	}

	return baseDir, nil
}

// CleanupIsolatedDirectory cleans up plugin's isolated directory
func (s *Sandbox) CleanupIsolatedDirectory(pluginID string) error {
	baseDir := filepath.Join(os.TempDir(), "plugin-sandbox", pluginID)
	return os.RemoveAll(baseDir)
}

// logViolation logs a security violation
func (s *Sandbox) logViolation(pluginID, violationType, details string) {
	// In production, this would log to a security monitoring system
	fmt.Printf("[SANDBOX VIOLATION] Plugin: %s, Type: %s, Details: %s\n",
		pluginID, violationType, details)
}

// GetActiveProcesses returns all active sandboxed processes
func (s *Sandbox) GetActiveProcesses() []*SandboxedProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()

	processes := make([]*SandboxedProcess, 0, len(s.activeProcs))
	for _, proc := range s.activeProcs {
		processes = append(processes, proc)
	}

	return processes
}

// GetProcessCount returns the number of active sandboxed processes
func (s *Sandbox) GetProcessCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.activeProcs)
}

// ValidatePluginAccess validates if plugin has access to resource
func (s *Sandbox) ValidatePluginAccess(pluginID, resource, action string) error {
	// Check filesystem access
	if resource == "filesystem" {
		if !s.IsFileSystemAccessAllowed(action) {
			return fmt.Errorf("filesystem access denied: %s", action)
		}
	}

	// Check network access
	if resource == "network" {
		if !s.IsNetworkAccessAllowed() {
			return fmt.Errorf("network access denied")
		}
	}

	return nil
}

// SetCustomLimits sets custom resource limits for a specific plugin
func (s *Sandbox) SetCustomLimits(limits *ResourceLimits) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = limits
}

// GetConfig returns the sandbox configuration
func (s *Sandbox) GetConfig() *PluginSandboxConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// UpdateConfig updates the sandbox configuration
func (s *Sandbox) UpdateConfig(config *PluginSandboxConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	s.config = config
	s.limits = &config.ResourceLimits

	return nil
}

// Cleanup cleans up all sandbox resources
func (s *Sandbox) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Kill all active processes
	for pid, proc := range s.activeProcs {
		proc.Monitoring = false
		if proc.Cmd.Process != nil {
			proc.Cmd.Process.Kill()
		}
		delete(s.activeProcs, pid)
	}

	return nil
}
