package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type AIProfile struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"` // gemini, openai, groq
	APIKey       string `json:"api_key"`
	DefaultModel string `json:"default_model"`
}

type DSProfile struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	DSN    string `json:"dsn"`
}

type Config struct {
	AIProfiles      []AIProfile `json:"ai_profiles"`
	DSProfiles      []DSProfile `json:"ds_profiles"`
	ActiveAIProfile string      `json:"active_ai_profile"`
	ActiveDSProfile string      `json:"active_ds_profile"`
	UserContext     string      `json:"user_context"`
	PrivacyMode     bool        `json:"privacy_mode"`
}

func GetConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mayo-cli")
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func GetModelsPath() string {
	return filepath.Join(GetConfigDir(), "models.txt")
}

func InitConfig() error {
	dir := GetConfigDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	sessionsDir := filepath.Join(dir, "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(sessionsDir, 0755); err != nil {
			return err
		}
	}

	dataDir := filepath.Join(dir, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return err
		}
	}

	viper.SetConfigFile(GetConfigPath())
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If file exists but is empty or invalid, create a default one
			SaveConfig(&Config{PrivacyMode: true})
		}
	} else {
		// Ensure PrivacyMode is initialized to true even for old configs
		if !viper.IsSet("privacy_mode") {
			viper.Set("privacy_mode", true)
			viper.WriteConfig()
		}
	}

	// Seed default models if doesn't exist
	modelsPath := GetModelsPath()
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		defaultModels := []string{
			"gemini-1.5-flash", "gemini-1.5-pro", "gemini-2.0-flash-exp",
			"gpt-4o", "gpt-4o-mini",
			"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "qwen-2.5-32b", "deepseek-v3",
			"claude-3-5-sonnet-20241022", "claude-3-opus-20240229", "claude-3-haiku-20240307",
		}
		var content strings.Builder
		for _, m := range defaultModels {
			content.WriteString(m + "\n")
		}
		os.WriteFile(modelsPath, []byte(content.String()), 0644)
	}

	return nil
}

func SaveConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetConfigPath(), data, 0644)
}

func LoadConfig() (*Config, error) {
	var cfg Config
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetModelList() []string {
	path := GetModelsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback to hardcoded defaults if file missing
		return []string{"gemini-1.5-flash", "gpt-4o", "llama-3.3-70b-versatile"}
	}

	var models []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		m := strings.TrimSpace(line)
		if m != "" && !strings.HasPrefix(m, "#") {
			models = append(models, m)
		}
	}
	return models
}
