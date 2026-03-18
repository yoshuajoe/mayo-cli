package enhancer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"mayo-cli/internal/config"
)

type Manager struct {
	mu           sync.RWMutex
	Enhancers    map[string]*EnhancerStatus `json:"enhancers"`
	Configs      map[string]*EnhancerConfig `json:"configs"`
	configDir    string
}

func NewManager() *Manager {
	m := &Manager{
		Enhancers: make(map[string]*EnhancerStatus),
		Configs:   make(map[string]*EnhancerConfig),
		configDir: config.GetConfigDir(),
	}
	m.Load()
	return m
}

func (m *Manager) GetStatePath() string {
	return filepath.Join(m.configDir, "enhancers.json")
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.GetStatePath(), data, 0644)
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.GetStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, m)
}

func (m *Manager) Register(config *EnhancerConfig, status *EnhancerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Configs[config.ID] = config
	m.Enhancers[status.ID] = status
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Configs, id)
	delete(m.Enhancers, id)
}

func (m *Manager) UpdateStatus(id string, update func(*EnhancerStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.Enhancers[id]; ok {
		update(s)
	}
}

func GetLogPath(id string) string {
	logDir := filepath.Join(config.GetConfigDir(), "logs", "enhancers")
	os.MkdirAll(logDir, 0755)
	return filepath.Join(logDir, id+".log")
}
