package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const (
	KeyringService = "mayo-cli"
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

func (p *AIProfile) GetAPIKey(useKeyring bool) string {
	if !useKeyring || p.APIKey != "[KEYRING]" {
		return p.APIKey
	}
	val, err := keyring.Get(KeyringService, p.Name)
	if err != nil {
		return ""
	}
	return val
}

func (p *AIProfile) SetAPIKey(key string, useKeyring bool) error {
	if !useKeyring {
		p.APIKey = key
		return nil
	}
	p.APIKey = "[KEYRING]"
	return keyring.Set(KeyringService, p.Name, key)
}

func (c *Config) GetTeleskopAPIKey() string {
	if !c.UseKeyring || c.TeleskopAPIKey != "[KEYRING]" {
		return c.TeleskopAPIKey
	}
	val, err := keyring.Get(KeyringService, "teleskop_id")
	if err != nil {
		return ""
	}
	return val
}

func (c *Config) SetTeleskopAPIKey(key string) error {
	if !c.UseKeyring {
		c.TeleskopAPIKey = key
		return nil
	}
	c.TeleskopAPIKey = "[KEYRING]"
	return keyring.Set(KeyringService, "teleskop_id", key)
}

type Config struct {
	AIProfiles      []AIProfile `json:"ai_profiles"`
	DSProfiles      []DSProfile `json:"ds_profiles"`
	ActiveAIProfile string      `json:"active_ai_profile"`
	ActiveDSProfile string      `json:"active_ds_profile"`
	UserContext     string      `json:"user_context"`
	PrivacyMode     bool        `json:"privacy_mode"`
	DefaultLimit    int         `json:"default_limit"` // Point 1.C
	UseKeyring      bool        `json:"use_keyring"`   // Point 1.B
	Interactive     bool        `json:"interactive"`   // Point 2.B
	TeleskopAPIKey  string      `json:"teleskop_api_key,omitempty"`
	AnalystEnabled  bool        `json:"analyst_enabled"`
	ServePort       int         `json:"serve_port,omitempty"`
	ServeToken      string      `json:"serve_token,omitempty"`
}

func GetConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mayo-cli")
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func GetModelsPath() string {
	return filepath.Join(GetConfigDir(), "models.yaml")
}

func GetReconcileDir() string {
	return filepath.Join(GetConfigDir(), "reconcile")
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

	reconDir := GetReconcileDir()
	if _, err := os.Stat(reconDir); os.IsNotExist(err) {
		if err := os.MkdirAll(reconDir, 0755); err != nil {
			return err
		}
	}

	viper.SetConfigFile(GetConfigPath())
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If file exists but is empty or invalid, create a default one
			SaveConfig(&Config{PrivacyMode: true, AnalystEnabled: true})
		}
	} else {
		// Ensure PrivacyMode is initialized to true even for old configs
		if !viper.IsSet("privacy_mode") {
			viper.Set("privacy_mode", true)
			viper.WriteConfig()
		}
		if !viper.IsSet("analyst_enabled") {
			viper.Set("analyst_enabled", true)
			viper.WriteConfig()
		}
	}

	// Seed default models if doesn't exist
	modelsPath := GetModelsPath()
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		defaultModels := `gemini:
  - gemini-2.5-pro
  - gemini-2.5-flash
  - gemini-2.5-flash-lite
  - gemini-3-pro-preview
  - gemini-3-flash-preview

openai:
  - gpt-5.4
  - gpt-5.4-mini
  - gpt-5.4-nano
  - gpt-4o
  - gpt-4o-mini
  - o3
  - o3-mini
  - o4-mini

groq:
  - llama-3.3-70b-versatile
  - llama-3.1-70b
  - llama-3.1-8b-instant
  - mixtral-8x7b
  - gemma-2-9b

deepseek:
  - deepseek-r1
  - deepseek-v3
  - deepseek-coder

qwen:
  - qwen-2.5-32b
  - qwen-2.5-72b

mistral:
  - mistral-nemo-12b

olmo:
  - olmo-2

anthropic:
  - claude-3-5-sonnet
  - claude-3-opus
  - claude-3-haiku`
		os.WriteFile(modelsPath, []byte(defaultModels), 0644)
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

func GetProviderList() []string {
	path := GetModelsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"gemini", "openai", "groq", "anthropic"}
	}

	var mData map[string][]string
	if err := yaml.Unmarshal(data, &mData); err != nil {
		return []string{"gemini", "openai", "groq", "anthropic"}
	}

	var providers []string
	for k := range mData {
		providers = append(providers, k)
	}
	sort.Strings(providers)
	return providers
}

func GetModelList(provider string) []string {
	path := GetModelsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback to minimal defaults if file missing
		switch provider {
		case "openai":
			return []string{"gpt-5.4", "gpt-4o"}
		case "gemini":
			return []string{"gemini-2.5-pro", "gemini-2.5-flash"}
		case "groq":
			return []string{"llama-3.3-70b-versatile"}
		default:
			return []string{"gemini-2.5-pro"}
		}
	}

	var mData map[string][]string
	if err := yaml.Unmarshal(data, &mData); err != nil {
		return []string{}
	}

	if models, ok := mData[provider]; ok {
		return models
	}

	// If provider not found directly, try lowercase
	if models, ok := mData[strings.ToLower(provider)]; ok {
		return models
	}

	return []string{}
}
