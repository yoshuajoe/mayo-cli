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
	PID         int       `json:"pid,omitempty"`
	ContainerID string    `json:"container_id,omitempty"`
	SessionID   string    `json:"session_id"`
	Port        int       `json:"port"`
	StartedAt   time.Time `json:"started_at"`
	IsDocker    bool      `json:"is_docker"`
	Version     string    `json:"version,omitempty"`
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
	args := []string{"serve", "--port", strconv.Itoa(port), "--spawned"}
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
		// Detach from current process group so it survives CLI termination
		// This works on Unix/Linux/macOS
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
		IsDocker:  false,
		Version:   "1.4.0", // Tracking version
	}
	m.register(s)

	return cmd.Process.Pid, nil
}

func (m *Manager) SpawnDocker(sessionID string, port int, token string) (string, error) {
	// 1. Port conflict check
	if m.IsPortInUse(port) {
		return "", fmt.Errorf("port %d is already in use. Please stop it first", port)
	}

	// 2. Prepare Docker command
	containerName := fmt.Sprintf("mayo-server-%d", port)
	configDir := config.GetConfigDir()
	
	// We use the image 'mayo-cli:latest' which should be built by the user or by Makefile
	args := []string{
		"run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:%d", port, port),
		"-v", fmt.Sprintf("%s:/root/.mayo", configDir),
		"mayo-cli:latest",
		"serve", "--port", strconv.Itoa(port), "--spawned",
	}

	if token != "" {
		args = append(args, "--token", token)
	}

	// Use terminal to run docker
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker error: %v (output: %s)", err, string(out))
	}

	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	displayID := sessionID
	if displayID == "" {
		displayID = "MASTER"
	}

	// 3. Register server
	s := RunningServer{
		ContainerID: containerID,
		SessionID:   displayID,
		Port:        port,
		StartedAt:   time.Now(),
		IsDocker:    true,
		Version:     "1.4.0",
	}
	m.register(s)

	return containerID, nil
}


func (m *Manager) List() []RunningServer {
	var active []RunningServer
	tracked := m.loadAll()
	
	// 1. Verify tracked ones first
	for _, s := range tracked {
		running := false
		if s.IsDocker {
			running = m.isContainerRunning(s.ContainerID)
		} else {
			running = m.isProcessRunning(s.PID, s.Port)
		}

		if running {
			active = append(active, s)
		}
	}

	// 2. Discover un-tracked Mayo processes on the system
	discovered := m.DiscoverProcesses()
	for _, ds := range discovered {
		// Only add if not already in the active list (matching by PID)
		isDuplicate := false
		for _, as := range active {
			if as.PID == ds.PID && as.PID != 0 {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			active = append(active, ds)
		}
	}

	if len(active) != len(tracked) {
		m.saveAll(active)
	}
	return active
}

func (m *Manager) DiscoverProcesses() []RunningServer {
	var found []RunningServer
	
	// Using ps to find all processes containing 'mayo' and 'serve'
	// This works across different versions as long as it has 'mayo' in binary name and 'serve' in args
	cmd := exec.Command("ps", "-A", "-o", "pid,command")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return found
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "ps -A") {
			continue
		}

		// Look for 'mayo' and 'serve' AND the unique flag '--spawned'
		lower := strings.ToLower(line)
		if strings.Contains(lower, "mayo") && strings.Contains(lower, "serve") && strings.Contains(lower, "--spawned") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				pid, _ := strconv.Atoi(parts[0])
				if pid == os.Getpid() {
					continue // Don't list ourselves if we are running 'serve' (unlikely for CLI but safe)
				}

				// Try to extract port from command line
				port := 8080 // Default
				for i, p := range parts {
					if p == "--port" && i+1 < len(parts) {
						if v, err := strconv.Atoi(parts[i+1]); err == nil {
							port = v
						}
					}
				}

				found = append(found, RunningServer{
					PID:       pid,
					Port:      port,
					SessionID: "UNKNOWN (DISCOVERED)",
					StartedAt: time.Now(),
				})
			}
		}
	}
	
	return found
}

func (m *Manager) StopAll() error {
	servers := m.List()
	if len(servers) == 0 {
		return fmt.Errorf("no active Mayo processes found to stop")
	}

	for _, s := range servers {
		if s.IsDocker {
			exec.Command("docker", "stop", s.ContainerID).Run()
			exec.Command("docker", "rm", s.ContainerID).Run()
		} else {
			proc, err := os.FindProcess(s.PID)
			if err == nil {
				proc.Signal(syscall.SIGTERM)
				time.Sleep(200 * time.Millisecond)
				proc.Kill() // Use SIGKILL to be absolutely sure
			}
		}
	}

	m.saveAll([]RunningServer{})
	return nil
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
		} else if s.SessionID == idOrSession || fmt.Sprintf("%d", s.PID) == idOrSession || s.ContainerID == idOrSession {
			matches = true
		}

		if matches {
			targets = append(targets, s)
		} else {
			remaining = append(remaining, s)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("server, session, or container not found")
	}

	for _, t := range targets {
		if t.IsDocker {
			// Check if any other sessions are still using this container
			lastForContainer := true
			for _, r := range remaining {
				if r.ContainerID == t.ContainerID {
					lastForContainer = false
					break
				}
			}

			if lastForContainer {
				// Stop and remove container
				exec.Command("docker", "stop", t.ContainerID).Run()
				exec.Command("docker", "rm", t.ContainerID).Run()
			}
		} else {
			// Standard process handling
			// Determine if we should kill the process
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

func (m *Manager) isProcessRunning(pid int, port int) bool {
	if pid == 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Check if process is alive (Unix only)
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	// Double check: Is something actually listening on that port?
	// This prevents 'ghost' processes from blocking new spawns
	if port > 0 {
		checkCmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t")
		out, _ := checkCmd.CombinedOutput()
		lsofPIDStr := strings.TrimSpace(string(out))
		if lsofPIDStr == "" {
			return false // No process listening on this port!
		}
		// Some lsof return multiple PIDs, check if ours is one of them
		found := false
		for _, p := range strings.Fields(lsofPIDStr) {
			if p == strconv.Itoa(pid) {
				found = true
				break
			}
		}
		return found
	}

	return true
}

func (m *Manager) isContainerRunning(containerID string) bool {
	if containerID == "" {
		return false
	}
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
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
