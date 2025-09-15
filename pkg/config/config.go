package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	Type              string        `mapstructure:"type"`               // 策略类型: lighter, binance, arbitrage, dynamic_hedge
	SpreadPercent     float64       `mapstructure:"spread_percent"`     // Binance价差百分比
	MonitorInterval   time.Duration `mapstructure:"monitor_interval"`   // 动态对冲监控间隔
	MaxLeverage       float64       `mapstructure:"max_leverage"`       // 最大杠杆率 (停止开仓)
	EmergencyLeverage float64       `mapstructure:"emergency_leverage"` // 紧急平仓杠杆率
	StopDuration      time.Duration `mapstructure:"stop_duration"`      // 停止开仓等待时间

	// 持续交易配置
	ContinuousMode  bool          `mapstructure:"continuous_mode"`  // 是否启用持续交易模式
	TradingInterval time.Duration `mapstructure:"trading_interval"` // 交易间隔
	VolumeTarget    float64       `mapstructure:"volume_target"`    // 日交易量目标 (USDT)
	MaxDailyTrades  int           `mapstructure:"max_daily_trades"` // 每日最大交易次数

	// 对冲平衡配置
	EnableHedgeBalancing bool          `mapstructure:"enable_hedge_balancing"` // 是否启用对冲平衡检查
	BalanceCheckInterval time.Duration `mapstructure:"balance_check_interval"` // 平衡检查间隔
	BalanceTolerance     float64       `mapstructure:"balance_tolerance"`      // 平衡容差百分比
	MinBalanceAdjust     float64       `mapstructure:"min_balance_adjust"`     // 最小平衡调整金额

	// 快速执行配置
	EnableFastExecution  bool          `mapstructure:"enable_fast_execution"`  // 是否启用快速执行
	FastCheckInterval    time.Duration `mapstructure:"fast_check_interval"`    // 快速检查间隔
	MaxExecutionDelay    time.Duration `mapstructure:"max_execution_delay"`    // 最大执行延迟
	EnablePreExecution   bool          `mapstructure:"enable_pre_execution"`   // 启用预执行
	PartialFillThreshold float64       `mapstructure:"partial_fill_threshold"` // 部分成交阈值
	MaxSlippagePercent   float64       `mapstructure:"max_slippage_percent"`   // 最大滑点百分比
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
	v.SetConfigType("yml")

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
	v.SetDefault("strategy.monitor_interval", 5*time.Second)
	v.SetDefault("strategy.max_leverage", 3.0)
	v.SetDefault("strategy.emergency_leverage", 5.0)
	v.SetDefault("strategy.stop_duration", 10*time.Minute)

	// 持续交易默认配置
	v.SetDefault("strategy.continuous_mode", true)
	v.SetDefault("strategy.trading_interval", 30*time.Second)
	v.SetDefault("strategy.volume_target", 100000.0) // 10万USDT日交易量目标
	v.SetDefault("strategy.max_daily_trades", 1000)  // 每日最大1000笔交易

	// 对冲平衡默认配置
	v.SetDefault("strategy.enable_hedge_balancing", true)
	v.SetDefault("strategy.balance_check_interval", 60*time.Second) // 每分钟检查一次平衡
	v.SetDefault("strategy.balance_tolerance", 5.0)                 // 5%容差
	v.SetDefault("strategy.min_balance_adjust", 50.0)               // 最小50U调整

	// 快速执行默认配置
	v.SetDefault("strategy.enable_fast_execution", true)
	v.SetDefault("strategy.fast_check_interval", 200*time.Millisecond) // 200ms高频检查
	v.SetDefault("strategy.max_execution_delay", 500*time.Millisecond) // 最大500ms延迟
	v.SetDefault("strategy.enable_pre_execution", true)                // 启用预执行
	v.SetDefault("strategy.partial_fill_threshold", 0.5)               // 50%部分成交阈值
	v.SetDefault("strategy.max_slippage_percent", 0.1)                 // 0.1%最大滑点

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
		"lighter":       true,
		"binance":       true,
		"arbitrage":     true,
		"dynamic_hedge": true,
	}
	if !validStrategies[c.Strategy.Type] {
		return fmt.Errorf("strategy.type must be one of: lighter, binance, arbitrage, dynamic_hedge")
	}

	// 根据策略类型验证相应的配置
	if c.Strategy.Type == "lighter" || c.Strategy.Type == "arbitrage" || c.Strategy.Type == "dynamic_hedge" {
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

	if c.Strategy.Type == "binance" || c.Strategy.Type == "arbitrage" || c.Strategy.Type == "dynamic_hedge" {
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
