package strategy

import (
	"math"
	"time"

	"go.uber.org/zap"
)

// RiskAction 风险行动类型
type RiskAction string

const (
	RiskActionContinueOpening RiskAction = "CONTINUE_OPENING" // 继续开仓
	RiskActionStopOpening     RiskAction = "STOP_OPENING"     // 停止开仓
	RiskActionStartClosing    RiskAction = "START_CLOSING"    // 开始平仓
	RiskActionEmergencyClose  RiskAction = "EMERGENCY_CLOSE"  // 紧急平仓
)

// String 返回风险行动的字符串表示
func (ra RiskAction) String() string {
	return string(ra)
}

// RiskStatus 风险状态
type RiskStatus struct {
	Action          RiskAction `json:"action"`           // 风险行动
	LighterLeverage float64    `json:"lighter_leverage"` // Lighter杠杆率
	BinanceLeverage float64    `json:"binance_leverage"` // Binance杠杆率
	MaxLeverage     float64    `json:"max_leverage"`     // 当前最高杠杆率
	Reason          string     `json:"reason"`           // 风控原因
	Timestamp       time.Time  `json:"timestamp"`
}

// CheckRisk 检查风险状态
func (rm *RiskManager) CheckRisk(pm *PositionManager) *RiskStatus {
	now := time.Now()

	lighterPositions := pm.GetLighterPositions()
	binancePositions := pm.GetBinancePositions()

	lighterLeverage := lighterPositions.Leverage
	binanceLeverage := binancePositions.Leverage
	maxLeverage := max(lighterLeverage, binanceLeverage)

	rm.logger.Debug("Risk check",
		zap.Float64("lighter_leverage", lighterLeverage),
		zap.Float64("binance_leverage", binanceLeverage),
		zap.Float64("max_leverage", maxLeverage),
	)

	status := &RiskStatus{
		LighterLeverage: lighterLeverage,
		BinanceLeverage: binanceLeverage,
		MaxLeverage:     maxLeverage,
		Timestamp:       now,
	}

	// 1. 检查紧急平仓条件 (5倍杠杆)
	if maxLeverage >= rm.config.EmergencyLeverage {
		status.Action = RiskActionEmergencyClose
		status.Reason = "Leverage exceeded emergency threshold"
		rm.logger.Error("Emergency close triggered",
			zap.Float64("max_leverage", maxLeverage),
			zap.Float64("emergency_threshold", rm.config.EmergencyLeverage),
		)
		return status
	}

	// 2. 检查停止开仓条件 (3倍杠杆)
	if maxLeverage >= rm.config.MaxLeverage {
		status.Action = RiskActionStopOpening
		status.Reason = "Leverage exceeded max threshold"
		rm.logger.Warn("Stop opening triggered",
			zap.Float64("max_leverage", maxLeverage),
			zap.Float64("max_threshold", rm.config.MaxLeverage),
		)

		// 检查是否需要开始平仓 (停止开仓10分钟后)
		if rm.shouldStartClosing(now) {
			status.Action = RiskActionStartClosing
			status.Reason = "Stop duration exceeded, starting to close positions"
			rm.logger.Info("Starting closing phase",
				zap.Duration("time_since_stop", now.Sub(rm.getLastStopTime())),
			)
		}

		return status
	}

	// 3. 检查是否有仓位需要平仓 (仓位为0后重新开始)
	if rm.allPositionsZero(pm) {
		status.Action = RiskActionContinueOpening
		status.Reason = "All positions are zero, ready to open new positions"
		rm.logger.Info("Ready to continue opening positions")
		return status
	}

	// 4. 正常开仓状态
	status.Action = RiskActionContinueOpening
	status.Reason = "Normal trading conditions"
	return status
}

// shouldStartClosing 检查是否应该开始平仓
func (rm *RiskManager) shouldStartClosing(now time.Time) bool {
	// TODO: 实现获取上次停止开仓时间的逻辑
	// 这里需要从strategy中获取lastStopTime
	return false
}

// getLastStopTime 获取上次停止开仓时间
func (rm *RiskManager) getLastStopTime() time.Time {
	// TODO: 实现获取上次停止时间的逻辑
	return time.Now()
}

// allPositionsZero 检查是否所有仓位都为0
func (rm *RiskManager) allPositionsZero(pm *PositionManager) bool {
	lighterPositions := pm.GetLighterPositions()
	binancePositions := pm.GetBinancePositions()

	// 检查Lighter仓位
	for _, pos := range lighterPositions.Positions {
		if pos.Size != 0 {
			return false
		}
	}

	// 检查Binance仓位
	for _, pos := range binancePositions.Positions {
		if pos.Size != 0 {
			return false
		}
	}

	return true
}

// GetPositionSummary 获取仓位摘要
func (pm *PositionManager) GetPositionSummary() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return map[string]interface{}{
		"lighter": map[string]interface{}{
			"exchange":   pm.lighterPositions.Exchange,
			"leverage":   pm.lighterPositions.Leverage,
			"positions":  pm.lighterPositions.Positions,
			"updated_at": pm.lighterPositions.UpdatedAt,
		},
		"binance": map[string]interface{}{
			"exchange":   pm.binancePositions.Exchange,
			"leverage":   pm.binancePositions.Leverage,
			"positions":  pm.binancePositions.Positions,
			"updated_at": pm.binancePositions.UpdatedAt,
		},
	}
}

// GetLighterPositions 获取Lighter仓位
func (pm *PositionManager) GetLighterPositions() *ExchangePositions {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.lighterPositions
}

// GetBinancePositions 获取Binance仓位
func (pm *PositionManager) GetBinancePositions() *ExchangePositions {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.binancePositions
}

// UpdateLighterPosition 更新Lighter仓位
func (pm *PositionManager) UpdateLighterPosition(symbol string, position *Position) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.lighterPositions.Positions == nil {
		pm.lighterPositions.Positions = make(map[string]*Position)
	}

	pm.lighterPositions.Positions[symbol] = position
	pm.lighterPositions.UpdatedAt = time.Now()

	pm.logger.Debug("Updated Lighter position",
		zap.String("symbol", symbol),
		zap.Float64("size", position.Size),
		zap.Float64("value", position.Value),
		zap.Float64("leverage", position.Leverage),
	)
}

// UpdateBinancePosition 更新Binance仓位
func (pm *PositionManager) UpdateBinancePosition(symbol string, position *Position) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.binancePositions.Positions == nil {
		pm.binancePositions.Positions = make(map[string]*Position)
	}

	pm.binancePositions.Positions[symbol] = position
	pm.binancePositions.UpdatedAt = time.Now()

	pm.logger.Debug("Updated Binance position",
		zap.String("symbol", symbol),
		zap.Float64("size", position.Size),
		zap.Float64("value", position.Value),
		zap.Float64("leverage", position.Leverage),
	)
}

// CalculateTotalLeverage 计算总杠杆率
func (pm *PositionManager) CalculateTotalLeverage() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 计算Lighter总杠杆率
	var lighterTotalValue float64
	for _, pos := range pm.lighterPositions.Positions {
		lighterTotalValue += math.Abs(pos.Value)
	}
	// TODO: 获取账户总资产来计算实际杠杆率
	pm.lighterPositions.Leverage = lighterTotalValue / 1000 // 假设账户资产为1000

	// 计算Binance总杠杆率
	var binanceTotalValue float64
	for _, pos := range pm.binancePositions.Positions {
		binanceTotalValue += math.Abs(pos.Value)
	}
	// TODO: 获取账户总资产来计算实际杠杆率
	pm.binancePositions.Leverage = binanceTotalValue / 1000 // 假设账户资产为1000

	pm.logger.Debug("Calculated total leverage",
		zap.Float64("lighter_leverage", pm.lighterPositions.Leverage),
		zap.Float64("binance_leverage", pm.binancePositions.Leverage),
	)
}

// max 返回两个float64中的最大值
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
