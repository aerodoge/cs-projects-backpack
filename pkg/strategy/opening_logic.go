package strategy

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// OpeningManager 开仓管理器
type OpeningManager struct {
	hedgeStrategy   *DynamicHedgeStrategy
	positionManager *PositionManager
	orderManager    *OrderManager
	orderMonitor    *OrderMonitor
	logger          *zap.Logger
}

// NewOpeningManager 创建开仓管理器
func NewOpeningManager(hedgeStrategy *DynamicHedgeStrategy) *OpeningManager {
	return &OpeningManager{
		hedgeStrategy:   hedgeStrategy,
		positionManager: hedgeStrategy.positionManager,
		orderManager:    hedgeStrategy.orderManager,
		orderMonitor:    hedgeStrategy.orderMonitor,
		logger:          hedgeStrategy.logger.Named("opening-manager"),
	}
}

// ExecuteOpeningLogic 执行开仓逻辑
func (om *OpeningManager) ExecuteOpeningLogic(ctx context.Context, config *DynamicHedgeConfig) error {
	om.logger.Debug("Starting opening logic execution")

	// 1. 获取当前仓位状态
	binancePositions := om.positionManager.GetBinancePositions()

	// 2. 确保BTC和ETH仓位存在
	btcPos := om.ensurePosition(binancePositions, "BTC")
	ethPos := om.ensurePosition(binancePositions, "ETH")

	// 3. 比较BTC和ETH仓位绝对值大小，选择仓位小的开仓
	btcAbsSize := math.Abs(btcPos.Size)
	ethAbsSize := math.Abs(ethPos.Size)

	var targetSymbol string
	var binanceSide string
	var lighterSide string

	if btcAbsSize <= ethAbsSize {
		// BTC仓位较小，开BTC仓位
		targetSymbol = "BTC"
		binanceSide = "SELL" // Binance做空BTC
		lighterSide = "BUY"  // Lighter做多BTC
		om.logger.Info("Selected BTC for opening",
			zap.Float64("btc_size", btcAbsSize),
			zap.Float64("eth_size", ethAbsSize),
		)
	} else {
		// ETH仓位较小，开ETH仓位
		targetSymbol = "ETH"
		binanceSide = "BUY"  // Binance做多ETH
		lighterSide = "SELL" // Lighter做空ETH
		om.logger.Info("Selected ETH for opening",
			zap.Float64("btc_size", btcAbsSize),
			zap.Float64("eth_size", ethAbsSize),
		)
	}

	// 4. 执行开仓流程：先Binance挂Maker单，成交后Lighter下Taker单
	return om.executeOpeningSequence(ctx, config, targetSymbol, binanceSide, lighterSide)
}

// ensurePosition 确保仓位结构存在
func (om *OpeningManager) ensurePosition(positions *ExchangePositions, symbol string) *Position {
	if pos, exists := positions.Positions[symbol]; exists {
		return pos
	}

	// 如果不存在，创建空仓位
	newPos := &Position{
		Symbol:   symbol,
		Size:     0,
		Value:    0,
		Leverage: 0,
	}
	positions.Positions[symbol] = newPos
	return newPos
}

// executeOpeningSequence 执行开仓序列
func (om *OpeningManager) executeOpeningSequence(
	ctx context.Context,
	config *DynamicHedgeConfig,
	symbol, binanceSide, lighterSide string,
) error {
	om.logger.Info("Executing opening sequence",
		zap.String("symbol", symbol),
		zap.String("binance_side", binanceSide),
		zap.String("lighter_side", lighterSide),
		zap.Float64("order_size", config.OrderSize),
	)

	// 1. 在Binance下Maker限价单
	binanceOrderID, err := om.placeBinanceMakerOrder(ctx, symbol, binanceSide, config)
	if err != nil {
		return fmt.Errorf("failed to place Binance maker order: %w", err)
	}

	// 2. 将订单添加到监控系统
	binanceOrder := &ActiveOrder{
		ID:        binanceOrderID,
		Exchange:  "binance",
		Symbol:    symbol,
		Side:      binanceSide,
		Size:      config.OrderSize,
		Status:    "PENDING",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	om.orderManager.AddOrder(binanceOrder)

	om.logger.Info("Binance maker order placed and added to monitoring",
		zap.String("order_id", binanceOrderID),
		zap.String("symbol", symbol),
		zap.String("side", binanceSide),
	)

	// 注意：Lighter的Taker单会在Binance订单成交时自动触发（通过OrderMonitor）

	return nil
}

// placeBinanceMakerOrder 在Binance下Maker限价单
func (om *OpeningManager) placeBinanceMakerOrder(
	ctx context.Context,
	symbol, side string,
	config *DynamicHedgeConfig,
) (string, error) {
	om.logger.Info("Placing Binance maker order",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("usdc_amount", config.OrderSize),
		zap.Float64("spread_percent", config.SpreadPercent),
	)

	// 根据symbol和side调用对应的Binance策略方法
	switch {
	case symbol == "BTC" && side == "SELL":
		// BTC空单
		order, err := om.hedgeStrategy.binanceStrategy.client.PlaceBTCShort(ctx, config.OrderSize, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	case symbol == "ETH" && side == "BUY":
		// ETH多单
		order, err := om.hedgeStrategy.binanceStrategy.client.PlaceETHLong(ctx, config.OrderSize, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	default:
		return "", fmt.Errorf("unsupported trading pair: %s %s", symbol, side)
	}
}

// PlaceLighterTakerOrder 在Lighter下Taker市价单（由OrderMonitor调用）
func (om *OpeningManager) PlaceLighterTakerOrder(
	ctx context.Context,
	symbol, side string,
	size float64,
) error {
	om.logger.Info("Placing Lighter taker order",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("usdt_amount", size),
	)

	// 将USDC金额转换为USDT金额（1:1汇率）
	usdtAmount := int64(size)
	leverage := 3 // 固定3倍杠杆

	// 根据symbol和side调用对应的Lighter策略方法
	switch {
	case symbol == "BTC" && side == "BUY":
		// BTC多单
		_, err := om.hedgeStrategy.lighterStrategy.client.PlaceBTCLong(ctx, usdtAmount, leverage)
		return err

	case symbol == "ETH" && side == "SELL":
		// ETH空单
		_, err := om.hedgeStrategy.lighterStrategy.client.PlaceETHShort(ctx, usdtAmount, leverage)
		return err

	default:
		return fmt.Errorf("unsupported Lighter trading pair: %s %s", symbol, side)
	}
}

// CheckOpeningConditions 检查开仓条件
func (om *OpeningManager) CheckOpeningConditions(config *DynamicHedgeConfig) (bool, string) {
	// 1. 检查杠杆率限制
	riskStatus := om.hedgeStrategy.riskManager.CheckRisk(om.positionManager)
	if riskStatus.MaxLeverage >= config.MaxLeverage {
		return false, fmt.Sprintf("leverage too high: %.2fx >= %.2fx",
			riskStatus.MaxLeverage, config.MaxLeverage)
	}

	// 2. 检查是否有未完成的订单
	activeOrders := om.orderManager.GetActiveOrders()
	if len(activeOrders) > 0 {
		return false, fmt.Sprintf("has %d active orders", len(activeOrders))
	}

	// 3. 检查账户余额（TODO: 实现具体的余额检查）

	return true, "all conditions met"
}

// GetOptimalOrderSize 获取最优订单大小
func (om *OpeningManager) GetOptimalOrderSize(config *DynamicHedgeConfig, symbol string) float64 {
	// 基础订单大小
	baseSize := config.OrderSize

	// TODO: 根据当前仓位情况动态调整订单大小
	// 例如：如果某个币种仓位已经很大，可以减少订单大小

	currentPositions := om.positionManager.GetBinancePositions()
	if pos, exists := currentPositions.Positions[symbol]; exists {
		positionRatio := math.Abs(pos.Size) / (baseSize * 10) // 假设最大仓位是10倍基础大小
		if positionRatio > 0.8 {
			// 如果仓位已经很大，减少订单大小
			baseSize *= (1 - positionRatio)
		}
	}

	om.logger.Debug("Calculated optimal order size",
		zap.String("symbol", symbol),
		zap.Float64("base_size", config.OrderSize),
		zap.Float64("optimal_size", baseSize),
	)

	return baseSize
}
