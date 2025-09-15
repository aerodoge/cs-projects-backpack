package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Lighter  LighterConfig  `mapstructure:"lighter"`
	Binance  BinanceConfig  `mapstructure:"binance"`
	Trading  TradingConfig  `mapstructure:"trading"`
	Strategy StrategyConfig `mapstructure:"strategy"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	App      AppConfig      `mapstructure:"app"`
}

type LighterConfig struct {
	APIKey       string `mapstructure:"api_key"`
	SecretKey    string `mapstructure:"secret_key"`
	PrivateKey   string `mapstructure:"private_key"`
	BaseURL      string `mapstructure:"base_url"`
	AccountIndex int64  `mapstructure:"account_index"`
	APIKeyIndex  uint8  `mapstructure:"api_key_index"`
	ChainID      uint32 `mapstructure:"chain_id"`
}

type BinanceConfig struct {
	APIKey    string `mapstructure:"api_key"`
	SecretKey string `mapstructure:"secret_key"`
	Testnet   bool   `mapstructure:"testnet"`
}

type TradingConfig struct {
	USDTAmount int64 `mapstructure:"usdt_amount"` // Lighter每次交易的USDT数量
	USDCAmount int64 `mapstructure:"usdc_amount"` // Binance每次交易的USDC数量
	Leverage   int   `mapstructure:"leverage"`    // 杠杆倍数
}

type StrategyConfig struct {
	Type          string  `mapstructure:"type"`           // 策略类型: lighter, binance, arbitrage
	SpreadPercent float64 `mapstructure:"spread_percent"` // Binance价差百分比
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Output     string `mapstructure:"output"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxAge     int    `mapstructure:"max_age"`
	MaxBackups int    `mapstructure:"max_backups"`
	Compress   bool   `mapstructure:"compress"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	Environment string `mapstructure:"environment"`
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")

	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	//v.AddConfigPath("$HOME/.lighter-trader")
	//v.AddConfigPath("/etc/lighter-trader")

	v.SetEnvPrefix("LIGHTER")
	v.AutomaticEnv()

	setDefaults(v)

	err := v.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var config Config
	err = v.Unmarshal(&config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("lighter.base_url", "https://api.lighter.xyz")
	v.SetDefault("lighter.chain_id", 1)
	v.SetDefault("lighter.account_index", 1)
	v.SetDefault("lighter.api_key_index", 0)

	v.SetDefault("binance.testnet", false)

	v.SetDefault("trading.usdt_amount", 1000)
	v.SetDefault("trading.usdc_amount", 1000)
	v.SetDefault("trading.leverage", 3)

	v.SetDefault("strategy.type", "arbitrage")
	v.SetDefault("strategy.spread_percent", 0.1)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.output", "logs/app.log")
	v.SetDefault("logging.max_size", 100)
	v.SetDefault("logging.max_age", 7)
	v.SetDefault("logging.max_backups", 3)
	v.SetDefault("logging.compress", true)

	v.SetDefault("app.name", "lighter-trader")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.environment", "production")
}

func (c *Config) GetLogDir() string {
	return filepath.Dir(c.Logging.Output)
}

func (c *Config) Validate() error {
	// 验证策略类型
	validStrategies := map[string]bool{
		"lighter":   true,
		"binance":   true,
		"arbitrage": true,
	}
	if !validStrategies[c.Strategy.Type] {
		return fmt.Errorf("strategy.type must be one of: lighter, binance, arbitrage")
	}

	// 根据策略类型验证相应的配置
	if c.Strategy.Type == "lighter" || c.Strategy.Type == "arbitrage" {
		if c.Lighter.APIKey == "" {
			return fmt.Errorf("lighter.api_key is required for %s strategy", c.Strategy.Type)
		}
		if c.Lighter.SecretKey == "" {
			return fmt.Errorf("lighter.secret_key is required for %s strategy", c.Strategy.Type)
		}
		if c.Lighter.PrivateKey == "" {
			return fmt.Errorf("lighter.private_key is required for %s strategy", c.Strategy.Type)
		}
	}

	if c.Strategy.Type == "binance" || c.Strategy.Type == "arbitrage" {
		if c.Binance.APIKey == "" {
			return fmt.Errorf("binance.api_key is required for %s strategy", c.Strategy.Type)
		}
		if c.Binance.SecretKey == "" {
			return fmt.Errorf("binance.secret_key is required for %s strategy", c.Strategy.Type)
		}
	}

	if c.Trading.USDTAmount <= 0 {
		return fmt.Errorf("trading.usdt_amount must be positive")
	}
	if c.Trading.USDCAmount <= 0 {
		return fmt.Errorf("trading.usdc_amount must be positive")
	}
	if c.Trading.Leverage <= 0 {
		return fmt.Errorf("trading.leverage must be positive")
	}
	if c.Strategy.SpreadPercent < 0 {
		return fmt.Errorf("strategy.spread_percent must be non-negative")
	}

	logDir := c.GetLogDir()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	return nil
}
