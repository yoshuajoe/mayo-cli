package teleskop

import (
	"encoding/json"
	"mayo-cli/internal/config"
	"os"
	"path/filepath"
	"sync"
)

type Manager struct {
	mu        sync.RWMutex
	Scrapers  map[string]*ScraperStatus `json:"scrapers"`
	configDir string
}

func NewManager() *Manager {
	m := &Manager{
		Scrapers:  make(map[string]*ScraperStatus),
		configDir: config.GetConfigDir(),
	}
	m.Load()
	return m
}

func (m *Manager) GetStatePath() string {
	return filepath.Join(m.configDir, "scrapers.json")
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
		return err
	}
	return json.Unmarshal(data, m)
}

func (m *Manager) RegisterScraper(status *ScraperStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Scrapers[status.ID] = status
}

func (m *Manager) RemoveScraper(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Scrapers, id)
}
