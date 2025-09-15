package main

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

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

	log.Info("Starting Lighter Exchange BTC-ETH Arbitrage Bot",
		zap.String("app_name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("environment", cfg.App.Environment),
	)

	if err := cfg.Validate(); err != nil {
		log.Fatal("Configuration validation failed", zap.Error(err))
	}

	log.Info("Configuration loaded successfully")

	client, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		log.Fatal("Failed to create Lighter client", zap.Error(err))
	}

	arbitrageStrategy := strategy.NewArbitrageStrategy(client)

	ctx := context.Background()

	arbitrageConfig := &strategy.ArbitrageConfig{
		USDTAmount: cfg.Trading.USDTAmount,
		Leverage:   cfg.Trading.Leverage,
	}

	err = arbitrageStrategy.ExecuteBTCETHArbitrage(ctx, arbitrageConfig)
	if err != nil {
		log.Fatal("Arbitrage strategy failed", zap.Error(err))
	}

	log.Info("Arbitrage strategy execution completed successfully")
}
