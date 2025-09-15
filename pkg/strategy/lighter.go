package strategy

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/lighter"
	"cs-projects-backpack/pkg/logger"
)

type LighterStrategy struct {
	client *lighter.Client
	logger *zap.Logger
}

type LighterConfig struct {
	USDTAmount int64 // 每次交易的USDT数量
	Leverage   int   // 杠杆倍数
}

func NewLighterStrategy(client *lighter.Client) *LighterStrategy {
	return &LighterStrategy{
		client: client,
		logger: logger.Named("lighter-strategy"),
	}
}

func (s *LighterStrategy) ExecuteBTCETHPair(ctx context.Context, config *LighterConfig) error {
	s.logger.Info("Starting Lighter BTC-ETH trading strategy",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)

	s.logger.Info("Placing BTC long order on Lighter",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)
	btcTx, err := s.client.PlaceBTCLong(ctx, config.USDTAmount, config.Leverage)
	if err != nil {
		s.logger.Error("Lighter BTC long order failed", zap.Error(err))
		return fmt.Errorf("lighter BTC多单失败: %w", err)
	}
	s.logger.Info("Lighter BTC long order successful", zap.String("tx_hash", btcTx.GetTxHash()))

	s.logger.Info("Placing ETH short order on Lighter",
		zap.Int64("usdt_amount", config.USDTAmount),
		zap.Int("leverage", config.Leverage),
	)
	ethTx, err := s.client.PlaceETHShort(ctx, config.USDTAmount, config.Leverage)
	if err != nil {
		s.logger.Error("Lighter ETH short order failed", zap.Error(err))
		return fmt.Errorf("lighter ETH空单失败: %w", err)
	}
	s.logger.Info("Lighter ETH short order successful", zap.String("tx_hash", ethTx.GetTxHash()))

	s.logger.Info("Lighter BTC-ETH trading completed successfully",
		zap.String("btc_position", "LONG with leverage"),
		zap.String("eth_position", "SHORT with leverage"),
	)

	return nil
}
