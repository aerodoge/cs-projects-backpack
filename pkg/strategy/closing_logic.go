package strategy

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// ClosingManager 平仓管理器
type ClosingManager struct {
	hedgeStrategy   *DynamicHedgeStrategy
	positionManager *PositionManager
	orderManager    *OrderManager
	orderMonitor    *OrderMonitor
	logger          *zap.Logger
}

// NewClosingManager 创建平仓管理器
func NewClosingManager(hedgeStrategy *DynamicHedgeStrategy) *ClosingManager {
	return &ClosingManager{
		hedgeStrategy:   hedgeStrategy,
		positionManager: hedgeStrategy.positionManager,
		orderManager:    hedgeStrategy.orderManager,
		orderMonitor:    hedgeStrategy.orderMonitor,
		logger:          hedgeStrategy.logger.Named("closing-manager"),
	}
}

// ExecuteClosingLogic 执行平仓逻辑
func (cm *ClosingManager) ExecuteClosingLogic(ctx context.Context, config *DynamicHedgeConfig) error {
	cm.logger.Info("Starting closing logic execution")

	// 1. 获取当前仓位状态
	binancePositions := cm.positionManager.GetBinancePositions()
	lighterPositions := cm.positionManager.GetLighterPositions()

	// 2. 检查是否所有仓位都已为0
	if cm.allPositionsZero(binancePositions, lighterPositions) {
		cm.logger.Info("All positions are zero, closing phase completed")
		return nil
	}

	// 3. 比较Binance中BTC和ETH仓位绝对值大小，选择仓位大的平仓
	btcPos := cm.ensurePosition(binancePositions, "BTC")
	ethPos := cm.ensurePosition(binancePositions, "ETH")

	btcAbsSize := math.Abs(btcPos.Size)
	ethAbsSize := math.Abs(ethPos.Size)

	var targetSymbol string
	var binanceSide string
	var lighterSide string

	if btcAbsSize >= ethAbsSize {
		// BTC仓位较大，优先平BTC仓位
		targetSymbol = "BTC"
		if btcPos.Size < 0 {
			// 当前是空头，平仓需要买入
			binanceSide = "BUY"
			lighterSide = "SELL" // 对应平掉Lighter的多头
		} else {
			// 当前是多头，平仓需要卖出
			binanceSide = "SELL"
			lighterSide = "BUY" // 对应平掉Lighter的空头
		}
		cm.logger.Info("Selected BTC for closing",
			zap.Float64("btc_size", btcAbsSize),
			zap.Float64("eth_size", ethAbsSize),
			zap.String("binance_side", binanceSide),
		)
	} else {
		// ETH仓位较大，优先平ETH仓位
		targetSymbol = "ETH"
		if ethPos.Size > 0 {
			// 当前是多头，平仓需要卖出
			binanceSide = "SELL"
			lighterSide = "BUY" // 对应平掉Lighter的空头
		} else {
			// 当前是空头，平仓需要买入
			binanceSide = "BUY"
			lighterSide = "SELL" // 对应平掉Lighter的多头
		}
		cm.logger.Info("Selected ETH for closing",
			zap.Float64("btc_size", btcAbsSize),
			zap.Float64("eth_size", ethAbsSize),
			zap.String("binance_side", binanceSide),
		)
	}

	// 4. 计算平仓数量（取当前仓位大小和标准订单大小的最小值）
	currentSize := math.Abs(btcAbsSize)
	if targetSymbol == "ETH" {
		currentSize = math.Abs(ethAbsSize)
	}

	closeSize := math.Min(currentSize, config.OrderSize)

	// 5. 执行平仓序列
	return cm.executeClosingSequence(ctx, config, targetSymbol, binanceSide, lighterSide, closeSize)
}

// ExecuteEmergencyClosing 执行紧急平仓
func (cm *ClosingManager) ExecuteEmergencyClosing(ctx context.Context, config *DynamicHedgeConfig) error {
	cm.logger.Error("Executing emergency closing due to high leverage")

	// 紧急平仓使用市价单，快速执行
	binancePositions := cm.positionManager.GetBinancePositions()
	lighterPositions := cm.positionManager.GetLighterPositions()

	// 平掉所有Binance仓位
	for symbol, pos := range binancePositions.Positions {
		if pos.Size != 0 {
			side := "BUY"
			if pos.Size > 0 {
				side = "SELL"
			}

			if err := cm.placeBinanceMarketOrder(ctx, symbol, side, math.Abs(pos.Size)); err != nil {
				cm.logger.Error("Failed to place emergency Binance order",
					zap.String("symbol", symbol),
					zap.Error(err),
				)
			}
		}
	}

	// 平掉所有Lighter仓位
	for symbol, pos := range lighterPositions.Positions {
		if pos.Size != 0 {
			side := "SELL"
			if pos.Size < 0 {
				side = "BUY"
			}

			if err := cm.placeLighterMarketOrder(ctx, symbol, side, math.Abs(pos.Size)); err != nil {
				cm.logger.Error("Failed to place emergency Lighter order",
					zap.String("symbol", symbol),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// executeClosingSequence 执行平仓序列
func (cm *ClosingManager) executeClosingSequence(
	ctx context.Context,
	config *DynamicHedgeConfig,
	symbol, binanceSide, lighterSide string,
	closeSize float64,
) error {
	cm.logger.Info("Executing closing sequence",
		zap.String("symbol", symbol),
		zap.String("binance_side", binanceSide),
		zap.String("lighter_side", lighterSide),
		zap.Float64("close_size", closeSize),
	)

	// 1. 在Binance下Maker限价单
	binanceOrderID, err := cm.placeBinanceClosingOrder(ctx, symbol, binanceSide, closeSize, config)
	if err != nil {
		return fmt.Errorf("failed to place Binance closing order: %w", err)
	}

	// 2. 将订单添加到监控系统
	binanceOrder := &ActiveOrder{
		ID:        binanceOrderID,
		Exchange:  "binance",
		Symbol:    symbol,
		Side:      binanceSide,
		Size:      closeSize,
		Status:    "PENDING",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	cm.orderManager.AddOrder(binanceOrder)

	cm.logger.Info("Binance closing order placed and added to monitoring",
		zap.String("order_id", binanceOrderID),
		zap.String("symbol", symbol),
		zap.String("side", binanceSide),
	)

	return nil
}

// placeBinanceClosingOrder 在Binance下平仓订单
func (cm *ClosingManager) placeBinanceClosingOrder(
	ctx context.Context,
	symbol, side string,
	size float64,
	config *DynamicHedgeConfig,
) (string, error) {
	cm.logger.Info("Placing Binance closing order",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("size", size),
		zap.Float64("spread_percent", config.SpreadPercent),
	)

	// 根据symbol和side调用对应的方法
	switch {
	case symbol == "BTC" && side == "BUY":
		// 平BTC空头（买入BTC）
		order, err := cm.hedgeStrategy.binanceStrategy.client.PlaceETHLong(ctx, size, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	case symbol == "BTC" && side == "SELL":
		// 平BTC多头（卖出BTC）
		order, err := cm.hedgeStrategy.binanceStrategy.client.PlaceBTCShort(ctx, size, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	case symbol == "ETH" && side == "BUY":
		// 平ETH空头（买入ETH）
		order, err := cm.hedgeStrategy.binanceStrategy.client.PlaceETHLong(ctx, size, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	case symbol == "ETH" && side == "SELL":
		// 平ETH多头（卖出ETH）
		order, err := cm.hedgeStrategy.binanceStrategy.client.PlaceBTCShort(ctx, size, config.SpreadPercent)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", order.OrderID), nil

	default:
		return "", fmt.Errorf("unsupported closing pair: %s %s", symbol, side)
	}
}

// placeBinanceMarketOrder 在Binance下市价单（紧急平仓用）
func (cm *ClosingManager) placeBinanceMarketOrder(ctx context.Context, symbol, side string, size float64) error {
	cm.logger.Warn("Placing Binance market order for emergency closing",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("size", size),
	)

	// TODO: 实现Binance市价单逻辑
	return nil
}

// placeLighterMarketOrder 在Lighter下市价单（紧急平仓用）
func (cm *ClosingManager) placeLighterMarketOrder(ctx context.Context, symbol, side string, size float64) error {
	cm.logger.Warn("Placing Lighter market order for emergency closing",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("size", size),
	)

	// TODO: 实现Lighter市价单逻辑
	return nil
}

// PlaceLighterClosingOrder 在Lighter下平仓订单（由OrderMonitor调用）
func (cm *ClosingManager) PlaceLighterClosingOrder(
	ctx context.Context,
	symbol, side string,
	size float64,
) error {
	cm.logger.Info("Placing Lighter closing order",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("usdt_amount", size),
	)

	// 将USDC金额转换为USDT金额（1:1汇率）
	usdtAmount := int64(size)
	leverage := 3 // 固定3倍杠杆

	// 根据symbol和side调用对应的Lighter策略方法
	switch {
	case symbol == "BTC" && side == "SELL":
		// 平BTC多头（卖出BTC）
		_, err := cm.hedgeStrategy.lighterStrategy.client.PlaceETHShort(ctx, usdtAmount, leverage)
		return err

	case symbol == "BTC" && side == "BUY":
		// 平BTC空头（买入BTC）
		_, err := cm.hedgeStrategy.lighterStrategy.client.PlaceBTCLong(ctx, usdtAmount, leverage)
		return err

	case symbol == "ETH" && side == "BUY":
		// 平ETH空头（买入ETH）
		_, err := cm.hedgeStrategy.lighterStrategy.client.PlaceBTCLong(ctx, usdtAmount, leverage)
		return err

	case symbol == "ETH" && side == "SELL":
		// 平ETH多头（卖出ETH）
		_, err := cm.hedgeStrategy.lighterStrategy.client.PlaceETHShort(ctx, usdtAmount, leverage)
		return err

	default:
		return fmt.Errorf("unsupported Lighter closing pair: %s %s", symbol, side)
	}
}

// ensurePosition 确保仓位结构存在
func (cm *ClosingManager) ensurePosition(positions *ExchangePositions, symbol string) *Position {
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

// allPositionsZero 检查是否所有仓位都为0
func (cm *ClosingManager) allPositionsZero(binancePos, lighterPos *ExchangePositions) bool {
	// 检查Binance仓位
	for _, pos := range binancePos.Positions {
		if pos.Size != 0 {
			return false
		}
	}

	// 检查Lighter仓位
	for _, pos := range lighterPos.Positions {
		if pos.Size != 0 {
			return false
		}
	}

	return true
}

// GetTotalPositionValue 获取总仓位价值
func (cm *ClosingManager) GetTotalPositionValue() (float64, float64) {
	binancePositions := cm.positionManager.GetBinancePositions()
	lighterPositions := cm.positionManager.GetLighterPositions()

	var binanceTotal, lighterTotal float64

	for _, pos := range binancePositions.Positions {
		binanceTotal += math.Abs(pos.Value)
	}

	for _, pos := range lighterPositions.Positions {
		lighterTotal += math.Abs(pos.Value)
	}

	return binanceTotal, lighterTotal
}

// CheckClosingConditions 检查平仓条件
func (cm *ClosingManager) CheckClosingConditions(config *DynamicHedgeConfig) (bool, string) {
	riskStatus := cm.hedgeStrategy.riskManager.CheckRisk(cm.positionManager)

	// 1. 检查是否达到停止开仓后的等待时间
	if riskStatus.MaxLeverage >= config.MaxLeverage {
		// TODO: 检查是否已经等待了足够的时间
		return true, "leverage limit reached and wait time exceeded"
	}

	// 2. 检查是否达到紧急平仓条件
	if riskStatus.MaxLeverage >= config.EmergencyLeverage {
		return true, "emergency leverage threshold exceeded"
	}

	return false, "closing conditions not met"
}
