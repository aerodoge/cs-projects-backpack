package main

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/binance"
	"cs-projects-backpack/pkg/config"
	"cs-projects-backpack/pkg/lighter"
	"cs-projects-backpack/pkg/logger"
	"cs-projects-backpack/pkg/strategy"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.Initialize(&cfg.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	log.Info("Starting Trading Bot",
		zap.String("app_name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("environment", cfg.App.Environment),
		zap.String("strategy_type", cfg.Strategy.Type),
	)

	if err := cfg.Validate(); err != nil {
		log.Fatal("Configuration validation failed", zap.Error(err))
	}

	log.Info("Configuration loaded successfully")

	ctx := context.Background()

	switch cfg.Strategy.Type {
	case "lighter":
		err = runLighterStrategy(ctx, cfg, log)
	case "binance":
		err = runBinanceStrategy(ctx, cfg, log)
	case "arbitrage":
		err = runArbitrageStrategy(ctx, cfg, log)
	default:
		log.Fatal("Unknown strategy type", zap.String("type", cfg.Strategy.Type))
	}

	if err != nil {
		log.Fatal("Strategy execution failed", zap.Error(err))
	}

	log.Info("Strategy execution completed successfully")
}

func runLighterStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Lighter Strategy ===")

	lighterClient, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		return fmt.Errorf("failed to create Lighter client: %w", err)
	}

	lighterStrategy := strategy.NewLighterStrategy(lighterClient)

	lighterConfig := &strategy.LighterConfig{
		USDTAmount: cfg.Trading.USDTAmount,
		Leverage:   cfg.Trading.Leverage,
	}

	return lighterStrategy.ExecuteBTCETHPair(ctx, lighterConfig)
}

func runBinanceStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Binance Strategy ===")

	binanceClient, err := binance.NewClient(&cfg.Binance)
	if err != nil {
		return fmt.Errorf("failed to create Binance client: %w", err)
	}

	binanceStrategy := strategy.NewBinanceStrategy(binanceClient)

	binanceConfig := &strategy.BinanceConfig{
		USDCAmount:    float64(cfg.Trading.USDCAmount),
		SpreadPercent: cfg.Strategy.SpreadPercent,
	}

	return binanceStrategy.ExecuteBTCETHPair(ctx, binanceConfig)
}

func runArbitrageStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Arbitrage Strategy ===")

	// Create Lighter client
	lighterClient, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		return fmt.Errorf("failed to create Lighter client: %w", err)
	}

	// Create Binance client
	binanceClient, err := binance.NewClient(&cfg.Binance)
	if err != nil {
		return fmt.Errorf("failed to create Binance client: %w", err)
	}

	// Create individual strategies
	lighterStrategy := strategy.NewLighterStrategy(lighterClient)
	binanceStrategy := strategy.NewBinanceStrategy(binanceClient)

	// Create arbitrage strategy
	arbitrageStrategy := strategy.NewArbitrageStrategy(lighterStrategy, binanceStrategy)

	arbitrageConfig := &strategy.ArbitrageConfig{
		USDTAmount:    cfg.Trading.USDTAmount,
		USDCAmount:    cfg.Trading.USDCAmount,
		Leverage:      cfg.Trading.Leverage,
		SpreadPercent: cfg.Strategy.SpreadPercent,
	}

	return arbitrageStrategy.ExecuteBTCETHArbitrage(ctx, arbitrageConfig)
}
