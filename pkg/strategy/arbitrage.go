package strategy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/logger"
)

type ArbitrageStrategy struct {
	lighterStrategy *LighterStrategy
	binanceStrategy *BinanceStrategy
	logger          *zap.Logger
}

type ArbitrageConfig struct {
	USDTAmount    int64   // Lighter每次交易的USDT数量
	USDCAmount    int64   // Binance每次交易的USDC数量
	Leverage      int     // Lighter杠杆倍数
	SpreadPercent float64 // Binance挂单价差百分比
}

func NewArbitrageStrategy(lighterStrategy *LighterStrategy, binanceStrategy *BinanceStrategy) *ArbitrageStrategy {
	return &ArbitrageStrategy{
		lighterStrategy: lighterStrategy,
		binanceStrategy: binanceStrategy,
		logger:          logger.Named("arbitrage-strategy"),
	}
}

func (s *ArbitrageStrategy) ExecuteBTCETHArbitrage(ctx context.Context, config *ArbitrageConfig) error {
	s.logger.Info("Starting BTC-ETH dual-exchange arbitrage strategy",
		zap.Int64("lighter_usdt_amount", config.USDTAmount),
		zap.Int64("binance_usdc_amount", config.USDCAmount),
		zap.Int("lighter_leverage", config.Leverage),
		zap.Float64("binance_spread_percent", config.SpreadPercent),
	)

	// Phase 1: Execute on Lighter exchange (Taker)
	s.logger.Info("=== Phase 1: Executing on Lighter exchange (Taker) ===")

	lighterConfig := &LighterConfig{
		USDTAmount: config.USDTAmount,
		Leverage:   config.Leverage,
	}

	err := s.lighterStrategy.ExecuteBTCETHPair(ctx, lighterConfig)
	if err != nil {
		s.logger.Error("Lighter strategy execution failed", zap.Error(err))
		return fmt.Errorf("lighter策略执行失败: %w", err)
	}

	// Phase 2: Execute opposite on Binance (Maker)
	s.logger.Info("=== Phase 2: Executing opposite on Binance (Maker) ===")

	time.Sleep(1 * time.Second)

	binanceConfig := &BinanceConfig{
		USDCAmount:    float64(config.USDCAmount),
		SpreadPercent: config.SpreadPercent,
	}

	err = s.binanceStrategy.ExecuteBTCETHPair(ctx, binanceConfig)
	if err != nil {
		s.logger.Error("Binance strategy execution failed", zap.Error(err))
		return fmt.Errorf("binance策略执行失败: %w", err)
	}

	// Summary
	s.logger.Info("=== Arbitrage strategy completed successfully ===")
	s.logger.Info("Positions summary",
		zap.String("lighter_btc", "LONG with leverage"),
		zap.String("lighter_eth", "SHORT with leverage"),
		zap.String("binance_btc", "SHORT as maker"),
		zap.String("binance_eth", "LONG as maker"),
	)

	return nil
}
