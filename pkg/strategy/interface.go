package strategy

import "context"

// TradingStrategy 定义通用交易策略接口
type TradingStrategy interface {
	ExecuteBTCETHPair(ctx context.Context, config interface{}) error
}

// StrategyType 定义策略类型
type StrategyType string

const (
	StrategyLighter      StrategyType = "lighter"
	StrategyBinance      StrategyType = "binance"
	StrategyArbitrage    StrategyType = "arbitrage"
	StrategyDynamicHedge StrategyType = "dynamic_hedge"
)

// GetStrategyName 获取策略名称
func (s StrategyType) String() string {
	return string(s)
}
