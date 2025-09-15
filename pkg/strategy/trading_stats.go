package strategy

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/logger"
)

// TradingStatsManager 交易统计管理器
type TradingStatsManager struct {
	stats  *TradingStats
	mu     sync.RWMutex
	logger *zap.Logger
}

// TradingStats 交易统计信息
type TradingStats struct {
	// 日统计
	DailyVolume    float64   `json:"daily_volume"`     // 日交易量 (USDT)
	DailyTrades    int       `json:"daily_trades"`     // 日交易次数
	DailyStartTime time.Time `json:"daily_start_time"` // 日统计开始时间

	// 总统计
	TotalVolume float64   `json:"total_volume"` // 总交易量
	TotalTrades int       `json:"total_trades"` // 总交易次数
	StartTime   time.Time `json:"start_time"`   // 策略开始时间

	// 当前状态
	LastTradeTime time.Time `json:"last_trade_time"` // 最后交易时间
	CurrentPhase  string    `json:"current_phase"`   // 当前阶段
	ActiveOrders  int       `json:"active_orders"`   // 活跃订单数

	// 性能指标
	AvgTradeSize   float64 `json:"avg_trade_size"`  // 平均交易大小
	TradeFrequency float64 `json:"trade_frequency"` // 交易频率 (次/小时)
	VolumeProgress float64 `json:"volume_progress"` // 日交易量完成进度 (%)
}

// NewTradingStatsManager 创建交易统计管理器
func NewTradingStatsManager() *TradingStatsManager {
	now := time.Now()
	return &TradingStatsManager{
		stats: &TradingStats{
			DailyStartTime: now,
			StartTime:      now,
			CurrentPhase:   "INITIALIZING",
		},
		logger: logger.Named("trading-stats"),
	}
}

// RecordTrade 记录交易
func (tsm *TradingStatsManager) RecordTrade(volume float64, tradeType string) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	now := time.Now()

	// 检查是否需要重置日统计
	if !tsm.isSameDay(now, tsm.stats.DailyStartTime) {
		tsm.resetDailyStats(now)
	}

	// 更新统计
	tsm.stats.DailyVolume += volume
	tsm.stats.DailyTrades++
	tsm.stats.TotalVolume += volume
	tsm.stats.TotalTrades++
	tsm.stats.LastTradeTime = now

	// 计算平均交易大小
	if tsm.stats.TotalTrades > 0 {
		tsm.stats.AvgTradeSize = tsm.stats.TotalVolume / float64(tsm.stats.TotalTrades)
	}

	// 计算交易频率 (次/小时)
	if duration := now.Sub(tsm.stats.StartTime); duration > 0 {
		hours := duration.Hours()
		if hours > 0 {
			tsm.stats.TradeFrequency = float64(tsm.stats.TotalTrades) / hours
		}
	}

	tsm.logger.Info("Trade recorded",
		zap.String("type", tradeType),
		zap.Float64("volume", volume),
		zap.Float64("daily_volume", tsm.stats.DailyVolume),
		zap.Int("daily_trades", tsm.stats.DailyTrades),
	)
}

// UpdatePhase 更新当前阶段
func (tsm *TradingStatsManager) UpdatePhase(phase string) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	oldPhase := tsm.stats.CurrentPhase
	tsm.stats.CurrentPhase = phase

	tsm.logger.Info("Phase updated",
		zap.String("old_phase", oldPhase),
		zap.String("new_phase", phase),
	)
}

// UpdateActiveOrders 更新活跃订单数
func (tsm *TradingStatsManager) UpdateActiveOrders(count int) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	tsm.stats.ActiveOrders = count
}

// UpdateVolumeProgress 更新交易量进度
func (tsm *TradingStatsManager) UpdateVolumeProgress(target float64) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	if target > 0 {
		tsm.stats.VolumeProgress = (tsm.stats.DailyVolume / target) * 100
		if tsm.stats.VolumeProgress > 100 {
			tsm.stats.VolumeProgress = 100
		}
	}
}

// GetStats 获取统计信息
func (tsm *TradingStatsManager) GetStats() *TradingStats {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	// 返回副本
	statsCopy := *tsm.stats
	return &statsCopy
}

// GetDailyStats 获取日统计
func (tsm *TradingStatsManager) GetDailyStats() map[string]interface{} {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	return map[string]interface{}{
		"daily_volume":     tsm.stats.DailyVolume,
		"daily_trades":     tsm.stats.DailyTrades,
		"daily_start_time": tsm.stats.DailyStartTime,
		"volume_progress":  tsm.stats.VolumeProgress,
		"avg_trade_size":   tsm.stats.AvgTradeSize,
		"trade_frequency":  tsm.stats.TradeFrequency,
	}
}

// CheckDailyTargets 检查日目标完成情况
func (tsm *TradingStatsManager) CheckDailyTargets(volumeTarget float64, tradesTarget int) (bool, bool) {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	volumeReached := tsm.stats.DailyVolume >= volumeTarget
	tradesReached := tsm.stats.DailyTrades >= tradesTarget

	return volumeReached, tradesReached
}

// ShouldPauseTradingForDay 检查是否应该暂停交易
func (tsm *TradingStatsManager) ShouldPauseTradingForDay(maxTrades int) bool {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	return tsm.stats.DailyTrades >= maxTrades
}

// GetTradeVelocity 获取交易速度 (交易/分钟)
func (tsm *TradingStatsManager) GetTradeVelocity() float64 {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	if tsm.stats.DailyTrades == 0 {
		return 0
	}

	dayDuration := time.Since(tsm.stats.DailyStartTime)
	if dayDuration.Minutes() == 0 {
		return 0
	}

	return float64(tsm.stats.DailyTrades) / dayDuration.Minutes()
}

// LogStats 输出统计日志
func (tsm *TradingStatsManager) LogStats() {
	stats := tsm.GetStats()

	tsm.logger.Info("Trading Statistics Summary",
		zap.Float64("daily_volume", stats.DailyVolume),
		zap.Int("daily_trades", stats.DailyTrades),
		zap.Float64("total_volume", stats.TotalVolume),
		zap.Int("total_trades", stats.TotalTrades),
		zap.String("current_phase", stats.CurrentPhase),
		zap.Int("active_orders", stats.ActiveOrders),
		zap.Float64("avg_trade_size", stats.AvgTradeSize),
		zap.Float64("trade_frequency", stats.TradeFrequency),
		zap.Float64("volume_progress", stats.VolumeProgress),
	)
}

// resetDailyStats 重置日统计
func (tsm *TradingStatsManager) resetDailyStats(newStartTime time.Time) {
	tsm.logger.Info("Resetting daily stats",
		zap.Float64("previous_daily_volume", tsm.stats.DailyVolume),
		zap.Int("previous_daily_trades", tsm.stats.DailyTrades),
	)

	tsm.stats.DailyVolume = 0
	tsm.stats.DailyTrades = 0
	tsm.stats.DailyStartTime = newStartTime
	tsm.stats.VolumeProgress = 0
}

// isSameDay 检查两个时间是否为同一天
func (tsm *TradingStatsManager) isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
