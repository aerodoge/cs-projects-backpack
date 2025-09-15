package strategy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/binance"
	"cs-projects-backpack/pkg/logger"
)

type BinanceStrategy struct {
	client *binance.Client
	logger *zap.Logger
}

type BinanceConfig struct {
	USDCAmount    float64 // 每次交易的USDC数量
	SpreadPercent float64 // 价差百分比
}

func NewBinanceStrategy(client *binance.Client) *BinanceStrategy {
	return &BinanceStrategy{
		client: client,
		logger: logger.Named("binance-strategy"),
	}
}

func (s *BinanceStrategy) ExecuteBTCETHPair(ctx context.Context, config *BinanceConfig) error {
	s.logger.Info("Starting Binance BTC-ETH trading strategy",
		zap.Float64("usdc_amount", config.USDCAmount),
		zap.Float64("spread_percent", config.SpreadPercent),
	)

	s.logger.Info("Placing BTC short order on Binance",
		zap.Float64("usdc_amount", config.USDCAmount),
		zap.Float64("spread_percent", config.SpreadPercent),
	)
	btcOrder, err := s.client.PlaceBTCShort(ctx, config.USDCAmount, config.SpreadPercent)
	if err != nil {
		s.logger.Error("Binance BTC short order failed", zap.Error(err))
		return fmt.Errorf("Binance BTC空单失败: %w", err)
	}
	s.logger.Info("Binance BTC short order successful", zap.Int64("order_id", btcOrder.OrderID))

	time.Sleep(1 * time.Second)

	s.logger.Info("Placing ETH long order on Binance",
		zap.Float64("usdc_amount", config.USDCAmount),
		zap.Float64("spread_percent", config.SpreadPercent),
	)
	ethOrder, err := s.client.PlaceETHLong(ctx, config.USDCAmount, config.SpreadPercent)
	if err != nil {
		s.logger.Error("Binance ETH long order failed", zap.Error(err))
		return fmt.Errorf("Binance ETH多单失败: %w", err)
	}
	s.logger.Info("Binance ETH long order successful", zap.Int64("order_id", ethOrder.OrderID))

	s.logger.Info("Binance BTC-ETH trading completed successfully",
		zap.String("btc_position", "SHORT as maker"),
		zap.String("eth_position", "LONG as maker"),
	)

	return nil
}
