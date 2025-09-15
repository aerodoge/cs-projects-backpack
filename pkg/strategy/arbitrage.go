package strategy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/lighter"
	"cs-projects-backpack/pkg/logger"
)

type ArbitrageStrategy struct {
	lighterClient *lighter.Client
	logger        *zap.Logger
}

type ArbitrageConfig struct {
	USDTAmount int64 // 每次交易的USDT数量
	Leverage   int   // 杠杆倍数
}

func NewArbitrageStrategy(lighterClient *lighter.Client) *ArbitrageStrategy {
	return &ArbitrageStrategy{
		lighterClient: lighterClient,
		logger:        logger.Named("arbitrage-strategy"),
	}
}

func (s *ArbitrageStrategy) ExecuteBTCETHArbitrage(ctx context.Context, config *ArbitrageConfig) error {
	s.logger.Info("Starting BTC-ETH arbitrage strategy",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)

	s.logger.Info("Executing on Lighter exchange with USDT-denominated orders")

	s.logger.Info("Placing BTC long order",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)
	btcTx, err := s.lighterClient.PlaceBTCLong(ctx, config.USDTAmount, config.Leverage)
	if err != nil {
		s.logger.Error("BTC long order failed", zap.Error(err))
		return fmt.Errorf("BTC多单失败: %w", err)
	}
	s.logger.Info("BTC long order successful", zap.String("tx_hash", btcTx.GetTxHash()))

	time.Sleep(1 * time.Second)

	s.logger.Info("Placing ETH short order",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)
	ethTx, err := s.lighterClient.PlaceETHShort(ctx, config.USDTAmount, config.Leverage)
	if err != nil {
		s.logger.Error("ETH short order failed", zap.Error(err))
		return fmt.Errorf("ETH空单失败: %w", err)
	}
	s.logger.Info("ETH short order successful", zap.String("tx_hash", ethTx.GetTxHash()))

	s.logger.Info("Arbitrage strategy completed successfully")
	s.logger.Warn("Remember to execute opposite operations on Binance to complete arbitrage",
		zap.String("recommendation", fmt.Sprintf("Short BTC and Long ETH on Binance with %d USDT and %dx leverage", config.USDTAmount, config.Leverage)),
	)

	return nil
}
