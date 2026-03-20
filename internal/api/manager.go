package api

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mayo-cli/internal/config"
)

type RunningServer struct {
	PID       int       `json:"pid"`
	SessionID string    `json:"session_id"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
}

type Manager struct {
	RootDir string
}

func NewManager() *Manager {
	dir := filepath.Join(config.GetConfigDir(), "api")
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	return &Manager{RootDir: dir}
}

func (m *Manager) Spawn(sessionID string, port int, token string) (int, error) {
	// 1. Port conflict check
	if m.IsPortInUse(port) {
		// Check if it's a mayo process we can reuse
		pid := m.GetPIDByPort(port)
		if pid > 0 {
			// It's a mayo process, we don't need to spawn, just register
			m.RegisterSession(sessionID, port, pid)
			return pid, nil
		}
		return 0, fmt.Errorf("port %d is already in use by another application. Please stop it first", port)
	}

	// 2. Prepare command
	exe, _ := os.Executable()
	args := []string{"serve", "--port", strconv.Itoa(port)}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	if token != "" {
		args = append(args, "--token", token)
	}

	displayID := sessionID
	if displayID == "" {
		displayID = "MASTER"
	}

	logFile := filepath.Join(m.RootDir, "logs", fmt.Sprintf("server_%d.log", port))
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(exe, args...)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Stdin = nil

	// 3. Start process in background
	if runtime.GOOS == "windows" {
		// Windows specific backgrounding if needed
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	if err := cmd.Start(); err != nil {
		f.Close()
		return 0, err
	}
	f.Close()

	// 4. Register server
	s := RunningServer{
		PID:       cmd.Process.Pid,
		SessionID: displayID,
		Port:      port,
		StartedAt: time.Now(),
	}
	m.register(s)

	return cmd.Process.Pid, nil
}


func (m *Manager) List() []RunningServer {
	var active []RunningServer
	all := m.loadAll()
	
	// Verify if still running
	for _, s := range all {
		if m.isProcessRunning(s.PID) {
			active = append(active, s)
		}
	}

	if len(active) != len(all) {
		m.saveAll(active)
	}
	return active
}

func (m *Manager) Stop(idOrSession string) error {
	all := m.loadAll()
	var remaining []RunningServer
	var targets []RunningServer

	isPort := false
	if _, err := strconv.Atoi(idOrSession); err == nil && len(idOrSession) < 6 {
		isPort = true
	}

	for _, s := range all {
		matches := false
		if isPort && strconv.Itoa(s.Port) == idOrSession {
			matches = true
		} else if s.SessionID == idOrSession || fmt.Sprintf("%d", s.PID) == idOrSession {
			matches = true
		}

		if matches {
			targets = append(targets, s)
		} else {
			remaining = append(remaining, s)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("server or session not found")
	}

	// Determine if we should kill the process
	for _, t := range targets {
		shouldKill := false
		if isPort {
			// If stopped by port, kill it
			shouldKill = true
		} else {
			// If stopped by session, check if any other sessions are still using this PID
			lastForPID := true
			for _, r := range remaining {
				if r.PID == t.PID {
					lastForPID = false
					break
				}
			}
			if lastForPID {
				shouldKill = true
			}
		}

		if shouldKill {
			// Kill process
			proc, err := os.FindProcess(t.PID)
			if err == nil {
				proc.Signal(syscall.SIGTERM)
				// Wait a bit and force kill if needed
				time.Sleep(200 * time.Millisecond)
				proc.Kill()
			}
		}
	}

	m.saveAll(remaining)
	return nil
}

func (m *Manager) RegisterSession(sessionID string, port int, pid int) {
	all := m.loadAll()
	// Prevent duplicate entries for the same session on same port
	for _, s := range all {
		if s.Port == port && s.SessionID == sessionID {
			return 
		}
	}

	s := RunningServer{
		PID:       pid,
		SessionID: sessionID,
		Port:      port,
		StartedAt: time.Now(),
	}
	all = append(all, s)
	m.saveAll(all)
}

func (m *Manager) GetByPort(port int) *RunningServer {
	// 1. Check tracked list
	all := m.List()
	for _, s := range all {
		if s.Port == port {
			return &s
		}
	}

	// 2. Fallback: Check system processes
	pid := m.GetPIDByPort(port)
	if pid > 0 {
		return &RunningServer{
			PID:       pid,
			Port:      port,
			SessionID: "MASTER",
			StartedAt: time.Now(),
		}
	}

	return nil
}

func (m *Manager) GetLogPath(port int) string {
	return filepath.Join(m.RootDir, "logs", fmt.Sprintf("server_%d.log", port))
}

func (m *Manager) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (m *Manager) IsPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	cmd := exec.Command("lsof", "-i", addr, "-sTCP:LISTEN")
	out, _ := cmd.CombinedOutput()
	return len(out) > 0
}

func (m *Manager) GetPIDByPort(port int) int {
	addr := fmt.Sprintf(":%d", port)
	// Get PID of process listening on port
	cmd := exec.Command("lsof", "-t", "-i", addr, "-sTCP:LISTEN")
	out, _ := cmd.CombinedOutput()
	if len(out) == 0 {
		return 0
	}

	pidStr := strings.TrimSpace(string(out))
	pid, _ := strconv.Atoi(pidStr)
	if pid <= 0 {
		return 0
	}

	// Verify it's a Mayo process
	cmd = exec.Command("ps", "-p", pidStr, "-o", "comm=")
	out, _ = cmd.CombinedOutput()
	comm := strings.ToLower(strings.TrimSpace(string(out)))
	if strings.Contains(comm, "mayo") {
		return pid
	}

	return 0
}


func (m *Manager) register(s RunningServer) {
	all := m.loadAll()
	all = append(all, s)
	m.saveAll(all)
}

func (m *Manager) loadAll() []RunningServer {
	path := filepath.Join(m.RootDir, "servers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return []RunningServer{}
	}
	var servers []RunningServer
	json.Unmarshal(data, &servers)
	return servers
}

func (m *Manager) saveAll(servers []RunningServer) {
	path := filepath.Join(m.RootDir, "servers.json")
	data, _ := json.MarshalIndent(servers, "", "  ")
	os.WriteFile(path, data, 0644)
}
