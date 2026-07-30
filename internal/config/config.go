// Package config provides centralized configuration for simplex-node.
package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"ParanoidX/internal/fileutil"
)

// Config holds all configuration for the node.
type Config struct {
	// Listen address for the HTTP server.
	Listen string `json:"listen,omitempty"`
	// DataDir is the directory for all state files.
	DataDir string `json:"data_dir,omitempty"`

	// VaultQuotaMB is the maximum vault size in MB.
	VaultQuotaMB int `json:"vault_quota_mb,omitempty"`

	// BillingPricesNg sets the price in ng for services.
	BillingPricesNg struct {
		InitSilverRound int64 `json:"init_silver_round,omitempty"`
		RwaRegister     int64 `json:"rwa_register,omitempty"`
		ChannelAccess   int64 `json:"channel_access,omitempty"`
	} `json:"billing_prices_ng,omitempty"`

	// BotEndpoints for role chat and alerts.
	IslandBotURL string `json:"island_bot_url,omitempty"`
	AlertURL     string `json:"alert_url,omitempty"`

	// TronTreasury address for USDT TRC20 deposits.
	TronTreasuryAddr string `json:"tron_treasury_addr,omitempty"`
	TronGridAPIKey   string `json:"tron_grid_api_key,omitempty"`

	// OllamaURL is the base URL for the local Ollama instance.
	OllamaURL string `json:"ollama_url,omitempty"`
	// OllamaModel is the model name to use (default: gemma4:latest).
	OllamaModel string `json:"ollama_model,omitempty"`
	// AskStewardToken is the Telegram bot token for AskSteward (optional).
	AskStewardToken string `json:"ask_steward_token,omitempty"`
	// DarkPushkinToken is the Telegram bot token for DarkPushkin (optional).
	DarkPushkinToken string `json:"dark_pushkin_token,omitempty"`
	// TorquemadaToken is the Telegram bot token for Torquemada (optional).
	TorquemadaToken string `json:"torquemada_token,omitempty"`
	// TorquemadaChatID is the chat ID for Torquemada notifications.
	TorquemadaChatID int64 `json:"torquemada_chat_id,omitempty"`

	// WhatsApp config for multi-platform gateway
	WhatsAppToken   string `json:"whatsapp_token,omitempty"`
	WhatsAppPhoneID string `json:"whatsapp_phone_id,omitempty"`
	WhatsAppAPIToken string `json:"whatsapp_api_token,omitempty"`

	// Signal config for multi-platform gateway
	SignalCLIPath string `json:"signal_cli_path,omitempty"`
	SignalNumber  string `json:"signal_number,omitempty"`

	// Matrix config for multi-platform gateway
	MatrixHomeserver string `json:"matrix_homeserver,omitempty"`
	MatrixUserID     string `json:"matrix_user_id,omitempty"`
	MatrixToken      string `json:"matrix_token,omitempty"`

	// Discord config for multi-platform gateway
	DiscordToken  string `json:"discord_token,omitempty"`
	DiscordAppID  string `json:"discord_app_id,omitempty"`
	DiscordGuildID string `json:"discord_guild_id,omitempty"`

	// AcestepURL is the base URL for the Acestep AI radio generator API.
	AcestepURL string `json:"acestep_url,omitempty"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	c := &Config{
		Listen:         "0.0.0.0:8080",
		DataDir:        filepath.Join(home, ".local/share/simplex-node"),
		VaultQuotaMB:   2048,
		IslandBotURL:   "http://127.0.0.1:5002/send",
		AlertURL:       "http://127.0.0.1:5002/send_alert",
		TronTreasuryAddr: "",
		TronGridAPIKey: "",
	}
	c.BillingPricesNg.InitSilverRound = 100000
	c.BillingPricesNg.RwaRegister = 50000
	c.BillingPricesNg.ChannelAccess = 10000
	return c
}

// Load reads config from path, merging with defaults.
// Missing fields use defaults.
func Load(path string) *Config {
	cfg := DefaultConfig()

	if path == "" {
		return cfg
	}

	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("config: no config file at %s, using defaults (%v)", path, err)
		return cfg
	}

	var fileCfg Config
	if err := json.Unmarshal(b, &fileCfg); err != nil {
		log.Printf("config: error parsing %s: %v, using defaults", path, err)
		return cfg
	}

	// Override defaults with file values
	if fileCfg.Listen != "" {
		cfg.Listen = fileCfg.Listen
	}
	if fileCfg.DataDir != "" {
		cfg.DataDir = fileCfg.DataDir
	}
	if fileCfg.VaultQuotaMB > 0 {
		cfg.VaultQuotaMB = fileCfg.VaultQuotaMB
	}
	if fileCfg.BillingPricesNg.InitSilverRound > 0 {
		cfg.BillingPricesNg.InitSilverRound = fileCfg.BillingPricesNg.InitSilverRound
	}
	if fileCfg.BillingPricesNg.RwaRegister > 0 {
		cfg.BillingPricesNg.RwaRegister = fileCfg.BillingPricesNg.RwaRegister
	}
	if fileCfg.BillingPricesNg.ChannelAccess > 0 {
		cfg.BillingPricesNg.ChannelAccess = fileCfg.BillingPricesNg.ChannelAccess
	}
	if fileCfg.IslandBotURL != "" {
		cfg.IslandBotURL = fileCfg.IslandBotURL
	}
	if fileCfg.AlertURL != "" {
		cfg.AlertURL = fileCfg.AlertURL
	}
	if fileCfg.TronTreasuryAddr != "" {
		cfg.TronTreasuryAddr = fileCfg.TronTreasuryAddr
	}
	if fileCfg.TronGridAPIKey != "" {
		cfg.TronGridAPIKey = fileCfg.TronGridAPIKey
	}
	if fileCfg.OllamaURL != "" {
		cfg.OllamaURL = fileCfg.OllamaURL
	}
	if fileCfg.OllamaModel != "" {
		cfg.OllamaModel = fileCfg.OllamaModel
	}
	if fileCfg.AskStewardToken != "" {
		cfg.AskStewardToken = fileCfg.AskStewardToken
	}
	if fileCfg.DarkPushkinToken != "" {
		cfg.DarkPushkinToken = fileCfg.DarkPushkinToken
	}
	if fileCfg.TorquemadaToken != "" {
		cfg.TorquemadaToken = fileCfg.TorquemadaToken
	}
	if fileCfg.TorquemadaChatID != 0 {
		cfg.TorquemadaChatID = fileCfg.TorquemadaChatID
	}

	if fileCfg.WhatsAppToken != "" {
		cfg.WhatsAppToken = fileCfg.WhatsAppToken
	}
	if fileCfg.WhatsAppPhoneID != "" {
		cfg.WhatsAppPhoneID = fileCfg.WhatsAppPhoneID
	}
	if fileCfg.WhatsAppAPIToken != "" {
		cfg.WhatsAppAPIToken = fileCfg.WhatsAppAPIToken
	}
	if fileCfg.SignalCLIPath != "" {
		cfg.SignalCLIPath = fileCfg.SignalCLIPath
	}
	if fileCfg.SignalNumber != "" {
		cfg.SignalNumber = fileCfg.SignalNumber
	}
	if fileCfg.MatrixHomeserver != "" {
		cfg.MatrixHomeserver = fileCfg.MatrixHomeserver
	}
	if fileCfg.MatrixUserID != "" {
		cfg.MatrixUserID = fileCfg.MatrixUserID
	}
	if fileCfg.MatrixToken != "" {
		cfg.MatrixToken = fileCfg.MatrixToken
	}
	if fileCfg.DiscordToken != "" {
		cfg.DiscordToken = fileCfg.DiscordToken
	}
	if fileCfg.DiscordAppID != "" {
		cfg.DiscordAppID = fileCfg.DiscordAppID
	}
	if fileCfg.DiscordGuildID != "" {
		cfg.DiscordGuildID = fileCfg.DiscordGuildID
	}

	return cfg
}

// Save writes the config to path atomically.
func (c *Config) Save(path string) error {
	return fileutil.WriteJSON(path, c)
}
