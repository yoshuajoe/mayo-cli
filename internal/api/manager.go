package api

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	// 1. Prepare command
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

	// 2. Start process in background
	if runtime.GOOS == "windows" {
		// Windows specific backgrounding if needed
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}

	if err := cmd.Start(); err != nil {
		f.Close()
		return 0, err
	}
	f.Close()

	// 3. Register server
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

func (m *Manager) Stop(idOrPort string) error {
	all := m.loadAll()
	var remaining []RunningServer
	var target *RunningServer

	for _, s := range all {
		if strconv.Itoa(s.Port) == idOrPort || s.SessionID == idOrPort || fmt.Sprintf("%d", s.PID) == idOrPort {
			target = &s
		} else {
			remaining = append(remaining, s)
		}
	}

	if target == nil {
		return fmt.Errorf("server not found")
	}

	// Kill process
	proc, err := os.FindProcess(target.PID)
	if err == nil {
		proc.Signal(syscall.SIGTERM)
		// Wait a bit and force kill if needed
		time.Sleep(500 * time.Millisecond)
		proc.Kill()
	}

	m.saveAll(remaining)
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
