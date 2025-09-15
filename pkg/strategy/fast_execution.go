package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// FastExecutionManager 快速执行管理器 - 优化Binance到Lighter的执行延迟
type FastExecutionManager struct {
	hedgeStrategy   *DynamicHedgeStrategy
	orderManager    *OrderManager
	positionManager *PositionManager
	logger          *zap.Logger

	// 执行配置
	config *FastExecutionConfig

	// 延迟统计
	executionStats *ExecutionStats
	mu             sync.RWMutex
}

// FastExecutionConfig 快速执行配置
type FastExecutionConfig struct {
	// 检查频率优化
	HighFrequencyMode bool          // 高频检查模式
	CheckInterval     time.Duration // 订单检查间隔 (默认200ms)
	MaxExecutionDelay time.Duration // 最大允许执行延迟 (默认500ms)

	// 预执行机制
	EnablePreExecution   bool    // 启用预执行 (部分成交即开始对冲)
	PartialFillThreshold float64 // 部分成交阈值 (50%即开始对冲)

	// 价格保护
	EnablePriceProtection bool          // 启用价格保护
	MaxSlippagePercent    float64       // 最大滑点百分比 (默认0.1%)
	PriceValidityWindow   time.Duration // 价格有效期窗口 (默认1秒)

	// 并发优化
	EnableConcurrentExecution bool // 启用并发执行
	MaxConcurrentOrders       int  // 最大并发订单数

	// 重试机制
	EnableRetry          bool          // 启用重试
	MaxRetryAttempts     int           // 最大重试次数
	RetryBackoffDuration time.Duration // 重试退避时间
}

// ExecutionStats 执行统计信息
type ExecutionStats struct {
	TotalExecutions      int64         `json:"total_executions"`
	SuccessfulExecutions int64         `json:"successful_executions"`
	FailedExecutions     int64         `json:"failed_executions"`
	AverageDelay         time.Duration `json:"average_delay"`
	MinDelay             time.Duration `json:"min_delay"`
	MaxDelay             time.Duration `json:"max_delay"`
	LastExecutionTime    time.Time     `json:"last_execution_time"`

	// 延迟分布
	DelayBuckets map[string]int64 `json:"delay_buckets"` // <100ms, 100-200ms, 200-500ms, >500ms
}

// ExecutionContext 执行上下文
type ExecutionContext struct {
	OrderID        string        `json:"order_id"`
	Symbol         string        `json:"symbol"`
	OriginalSide   string        `json:"original_side"`
	HedgeSide      string        `json:"hedge_side"`
	Size           float64       `json:"size"`
	OriginalPrice  float64       `json:"original_price"`
	ExecutionPrice float64       `json:"execution_price"`
	StartTime      time.Time     `json:"start_time"`
	DetectionTime  time.Time     `json:"detection_time"`
	ExecutionTime  time.Time     `json:"execution_time"`
	CompletionTime time.Time     `json:"completion_time"`
	TotalDelay     time.Duration `json:"total_delay"`
	Success        bool          `json:"success"`
	ErrorMessage   string        `json:"error_message,omitempty"`
}

// NewFastExecutionManager 创建快速执行管理器
func NewFastExecutionManager(hedgeStrategy *DynamicHedgeStrategy) *FastExecutionManager {
	return &FastExecutionManager{
		hedgeStrategy:   hedgeStrategy,
		orderManager:    hedgeStrategy.orderManager,
		positionManager: hedgeStrategy.positionManager,
		logger:          hedgeStrategy.logger.Named("fast-execution"),
		config:          NewDefaultFastExecutionConfig(),
		executionStats:  NewExecutionStats(),
	}
}

// NewDefaultFastExecutionConfig 创建默认快速执行配置
func NewDefaultFastExecutionConfig() *FastExecutionConfig {
	return &FastExecutionConfig{
		HighFrequencyMode:         true,
		CheckInterval:             200 * time.Millisecond,
		MaxExecutionDelay:         500 * time.Millisecond,
		EnablePreExecution:        true,
		PartialFillThreshold:      0.5, // 50%
		EnablePriceProtection:     true,
		MaxSlippagePercent:        0.1, // 0.1%
		PriceValidityWindow:       1 * time.Second,
		EnableConcurrentExecution: true,
		MaxConcurrentOrders:       3,
		EnableRetry:               true,
		MaxRetryAttempts:          3,
		RetryBackoffDuration:      100 * time.Millisecond,
	}
}

// NewExecutionStats 创建执行统计
func NewExecutionStats() *ExecutionStats {
	return &ExecutionStats{
		DelayBuckets: map[string]int64{
			"<100ms":    0,
			"100-200ms": 0,
			"200-500ms": 0,
			">500ms":    0,
		},
		MinDelay: time.Hour, // 初始化为一个大值
	}
}

// ExecuteFastHedge 快速执行对冲交易
func (fem *FastExecutionManager) ExecuteFastHedge(
	ctx context.Context,
	orderID, symbol, originalSide string,
	size, originalPrice float64,
) (*ExecutionContext, error) {
	execCtx := &ExecutionContext{
		OrderID:       orderID,
		Symbol:        symbol,
		OriginalSide:  originalSide,
		Size:          size,
		OriginalPrice: originalPrice,
		StartTime:     time.Now(),
	}

	fem.logger.Info("Starting fast hedge execution",
		zap.String("order_id", orderID),
		zap.String("symbol", symbol),
		zap.String("side", originalSide),
		zap.Float64("size", size),
		zap.Float64("price", originalPrice),
	)

	// 1. 确定对冲方向
	hedgeSide := fem.determineHedgeSide(symbol, originalSide)
	execCtx.HedgeSide = hedgeSide

	// 2. 价格保护检查
	if fem.config.EnablePriceProtection {
		if err := fem.validatePrice(ctx, symbol, originalPrice); err != nil {
			execCtx.Success = false
			execCtx.ErrorMessage = fmt.Sprintf("price validation failed: %v", err)
			return execCtx, err
		}
	}

	execCtx.DetectionTime = time.Now()

	// 3. 执行对冲交易
	executionPrice, err := fem.executeHedgeWithRetry(ctx, execCtx)
	if err != nil {
		execCtx.Success = false
		execCtx.ErrorMessage = err.Error()
		fem.updateStats(execCtx)
		return execCtx, err
	}

	execCtx.ExecutionPrice = executionPrice
	execCtx.ExecutionTime = time.Now()
	execCtx.CompletionTime = time.Now()
	execCtx.TotalDelay = execCtx.CompletionTime.Sub(execCtx.StartTime)
	execCtx.Success = true

	// 4. 更新统计信息
	fem.updateStats(execCtx)

	fem.logger.Info("Fast hedge execution completed",
		zap.String("order_id", orderID),
		zap.Duration("total_delay", execCtx.TotalDelay),
		zap.Float64("execution_price", executionPrice),
		zap.Bool("success", true),
	)

	return execCtx, nil
}

// determineHedgeSide 确定对冲方向
func (fem *FastExecutionManager) determineHedgeSide(symbol, originalSide string) string {
	// Binance成交 -> Lighter对冲
	// BTC: Binance空 -> Lighter多
	// ETH: Binance多 -> Lighter空
	switch {
	case symbol == "BTC" && originalSide == "SELL":
		return "BUY" // Lighter做多BTC
	case symbol == "ETH" && originalSide == "BUY":
		return "SELL" // Lighter做空ETH
	default:
		fem.logger.Warn("Unexpected trading pair for hedge",
			zap.String("symbol", symbol),
			zap.String("side", originalSide),
		)
		return originalSide // 默认同方向
	}
}

// validatePrice 验证价格有效性
func (fem *FastExecutionManager) validatePrice(ctx context.Context, symbol string, price float64) error {
	// TODO: 实现实时价格获取和验证
	// 1. 获取当前市场价格
	// 2. 计算价格偏差
	// 3. 检查是否在可接受滑点范围内

	fem.logger.Debug("Validating execution price",
		zap.String("symbol", symbol),
		zap.Float64("price", price),
		zap.Float64("max_slippage", fem.config.MaxSlippagePercent),
	)

	return nil // 暂时通过验证
}

// executeHedgeWithRetry 带重试的对冲执行
func (fem *FastExecutionManager) executeHedgeWithRetry(ctx context.Context, execCtx *ExecutionContext) (float64, error) {
	var lastErr error

	for attempt := 1; attempt <= fem.config.MaxRetryAttempts; attempt++ {
		executionPrice, err := fem.executeLighterHedge(ctx, execCtx)
		if err == nil {
			return executionPrice, nil
		}

		lastErr = err
		fem.logger.Warn("Hedge execution attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", fem.config.MaxRetryAttempts),
			zap.Error(err),
		)

		// 如果不是最后一次尝试，等待后重试
		if attempt < fem.config.MaxRetryAttempts {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(fem.config.RetryBackoffDuration * time.Duration(attempt)):
				// 指数退避
			}
		}
	}

	return 0, fmt.Errorf("hedge execution failed after %d attempts: %w", fem.config.MaxRetryAttempts, lastErr)
}

// executeLighterHedge 在Lighter执行对冲交易
func (fem *FastExecutionManager) executeLighterHedge(ctx context.Context, execCtx *ExecutionContext) (float64, error) {
	fem.logger.Info("Executing Lighter hedge with optimized parameters",
		zap.String("symbol", execCtx.Symbol),
		zap.String("side", execCtx.HedgeSide),
		zap.Float64("size", execCtx.Size),
	)

	usdtAmount := int64(execCtx.Size)
	leverage := 3 // 固定3倍杠杆

	// 根据symbol和side选择对应的交易方法
	switch {
	case execCtx.Symbol == "BTC" && execCtx.HedgeSide == "BUY":
		// BTC多单
		order, err := fem.hedgeStrategy.lighterStrategy.client.PlaceBTCLong(ctx, usdtAmount, leverage)
		if err != nil {
			return 0, fmt.Errorf("failed to place BTC long on Lighter: %w", err)
		}
		return float64(order.Price), nil

	case execCtx.Symbol == "ETH" && execCtx.HedgeSide == "SELL":
		// ETH空单
		order, err := fem.hedgeStrategy.lighterStrategy.client.PlaceETHShort(ctx, usdtAmount, leverage)
		if err != nil {
			return 0, fmt.Errorf("failed to place ETH short on Lighter: %w", err)
		}
		return float64(order.Price), nil

	default:
		return 0, fmt.Errorf("unsupported Lighter hedge trading pair: %s %s", execCtx.Symbol, execCtx.HedgeSide)
	}
}

// updateStats 更新执行统计
func (fem *FastExecutionManager) updateStats(execCtx *ExecutionContext) {
	fem.mu.Lock()
	defer fem.mu.Unlock()

	stats := fem.executionStats
	stats.TotalExecutions++
	stats.LastExecutionTime = execCtx.CompletionTime

	if execCtx.Success {
		stats.SuccessfulExecutions++

		delay := execCtx.TotalDelay

		// 更新延迟统计
		if delay < stats.MinDelay {
			stats.MinDelay = delay
		}
		if delay > stats.MaxDelay {
			stats.MaxDelay = delay
		}

		// 更新平均延迟
		if stats.SuccessfulExecutions == 1 {
			stats.AverageDelay = delay
		} else {
			// 增量更新平均值
			stats.AverageDelay = time.Duration(
				(int64(stats.AverageDelay)*int64(stats.SuccessfulExecutions-1) + int64(delay)) /
					int64(stats.SuccessfulExecutions),
			)
		}

		// 更新延迟分布
		switch {
		case delay < 100*time.Millisecond:
			stats.DelayBuckets["<100ms"]++
		case delay < 200*time.Millisecond:
			stats.DelayBuckets["100-200ms"]++
		case delay < 500*time.Millisecond:
			stats.DelayBuckets["200-500ms"]++
		default:
			stats.DelayBuckets[">500ms"]++
		}
	} else {
		stats.FailedExecutions++
	}

	// 记录统计日志
	fem.logger.Debug("Execution stats updated",
		zap.Int64("total", stats.TotalExecutions),
		zap.Int64("successful", stats.SuccessfulExecutions),
		zap.Int64("failed", stats.FailedExecutions),
		zap.Duration("avg_delay", stats.AverageDelay),
		zap.Duration("current_delay", execCtx.TotalDelay),
	)
}

// GetExecutionStats 获取执行统计
func (fem *FastExecutionManager) GetExecutionStats() *ExecutionStats {
	fem.mu.RLock()
	defer fem.mu.RUnlock()

	// 返回副本
	stats := &ExecutionStats{
		TotalExecutions:      fem.executionStats.TotalExecutions,
		SuccessfulExecutions: fem.executionStats.SuccessfulExecutions,
		FailedExecutions:     fem.executionStats.FailedExecutions,
		AverageDelay:         fem.executionStats.AverageDelay,
		MinDelay:             fem.executionStats.MinDelay,
		MaxDelay:             fem.executionStats.MaxDelay,
		LastExecutionTime:    fem.executionStats.LastExecutionTime,
		DelayBuckets:         make(map[string]int64),
	}

	for k, v := range fem.executionStats.DelayBuckets {
		stats.DelayBuckets[k] = v
	}

	return stats
}

// UpdateConfig 更新执行配置
func (fem *FastExecutionManager) UpdateConfig(config *FastExecutionConfig) {
	fem.mu.Lock()
	defer fem.mu.Unlock()

	fem.config = config
	fem.logger.Info("Fast execution config updated",
		zap.Duration("check_interval", config.CheckInterval),
		zap.Duration("max_delay", config.MaxExecutionDelay),
		zap.Bool("high_frequency", config.HighFrequencyMode),
		zap.Bool("pre_execution", config.EnablePreExecution),
		zap.Float64("partial_threshold", config.PartialFillThreshold),
	)
}

// IsDelayExcessive 检查延迟是否过大
func (fem *FastExecutionManager) IsDelayExcessive(delay time.Duration) bool {
	return delay > fem.config.MaxExecutionDelay
}

// LogPerformanceMetrics 记录性能指标
func (fem *FastExecutionManager) LogPerformanceMetrics() {
	stats := fem.GetExecutionStats()

	fem.logger.Info("Fast execution performance metrics",
		zap.Int64("total_executions", stats.TotalExecutions),
		zap.Int64("successful_executions", stats.SuccessfulExecutions),
		zap.Float64("success_rate", float64(stats.SuccessfulExecutions)/float64(stats.TotalExecutions)*100),
		zap.Duration("average_delay", stats.AverageDelay),
		zap.Duration("min_delay", stats.MinDelay),
		zap.Duration("max_delay", stats.MaxDelay),
		zap.Any("delay_distribution", stats.DelayBuckets),
	)
}
