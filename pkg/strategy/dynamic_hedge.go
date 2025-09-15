package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/logger"
)

// DynamicHedgeStrategy 动态对冲策略
type DynamicHedgeStrategy struct {
	lighterStrategy      *LighterStrategy
	binanceStrategy      *BinanceStrategy
	positionManager      *PositionManager
	orderManager         *OrderManager
	orderMonitor         *OrderMonitor
	riskManager          *RiskManager
	openingManager       *OpeningManager
	closingManager       *ClosingManager
	statsManager         *TradingStatsManager
	hedgeBalancer        *HedgeBalancer
	fastExecutionManager *FastExecutionManager
	logger               *zap.Logger

	// 策略状态
	isRunning     bool
	currentPhase  string // OPENING, CLOSING, STOPPED
	mu            sync.RWMutex
	stopChan      chan struct{}
	lastStopTime  time.Time
	lastTradeTime time.Time
}

// DynamicHedgeConfig 动态对冲配置
type DynamicHedgeConfig struct {
	OrderSize         float64       // 每次下单规模 (1000U)
	MaxLeverage       float64       // 最大杠杆率 (3倍停止开仓)
	EmergencyLeverage float64       // 紧急平仓杠杆率 (5倍)
	StopDuration      time.Duration // 停止开仓后等待时间 (10分钟)
	MonitorInterval   time.Duration // 监控间隔
	SpreadPercent     float64       // Binance价差百分比

	// 持续交易配置
	ContinuousMode  bool          // 是否启用持续交易模式
	TradingInterval time.Duration // 交易间隔 (每次交易后等待时间)
	VolumeTarget    float64       // 日交易量目标 (USDT)
	MaxDailyTrades  int           // 每日最大交易次数

	// 对冲平衡配置
	EnableHedgeBalancing bool          // 是否启用对冲平衡检查
	BalanceCheckInterval time.Duration // 平衡检查间隔
	BalanceTolerance     float64       // 平衡容差百分比
	MinBalanceAdjust     float64       // 最小平衡调整金额

	// 快速执行配置
	EnableFastExecution  bool          // 是否启用快速执行
	FastCheckInterval    time.Duration // 快速检查间隔
	MaxExecutionDelay    time.Duration // 最大执行延迟
	EnablePreExecution   bool          // 启用预执行 (部分成交即对冲)
	PartialFillThreshold float64       // 部分成交阈值
	MaxSlippagePercent   float64       // 最大滑点百分比
}

// Position 仓位信息
type Position struct {
	Symbol   string  `json:"symbol"`   // BTC, ETH
	Size     float64 `json:"size"`     // 仓位大小 (正数做多，负数做空)
	Value    float64 `json:"value"`    // 仓位价值 (USDT/USDC)
	Leverage float64 `json:"leverage"` // 杠杆率
}

// ExchangePositions 交易所仓位
type ExchangePositions struct {
	Exchange  string               `json:"exchange"`
	Positions map[string]*Position `json:"positions"` // symbol -> position
	Leverage  float64              `json:"leverage"`  // 总杠杆率
	UpdatedAt time.Time            `json:"updated_at"`
}

// PositionManager 仓位管理器
type PositionManager struct {
	lighterPositions *ExchangePositions
	binancePositions *ExchangePositions
	mu               sync.RWMutex
	logger           *zap.Logger
}

// OrderManager 订单管理器
type OrderManager struct {
	activeOrders map[string]*ActiveOrder // orderID -> order
	mu           sync.RWMutex
	logger       *zap.Logger
}

// ActiveOrder 活跃订单
type ActiveOrder struct {
	ID         string    `json:"id"`
	Exchange   string    `json:"exchange"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"` // BUY, SELL
	Size       float64   `json:"size"`
	Price      float64   `json:"price"`
	Status     string    `json:"status"` // PENDING, PARTIAL, FILLED, CANCELLED
	FilledSize float64   `json:"filled_size"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RiskManager 风控管理器
type RiskManager struct {
	config *DynamicHedgeConfig
	logger *zap.Logger
}

func NewDynamicHedgeStrategy(
	lighterStrategy *LighterStrategy,
	binanceStrategy *BinanceStrategy,
) *DynamicHedgeStrategy {
	strategy := &DynamicHedgeStrategy{
		lighterStrategy: lighterStrategy,
		binanceStrategy: binanceStrategy,
		positionManager: NewPositionManager(),
		orderManager:    NewOrderManager(),
		riskManager:     NewRiskManager(),
		statsManager:    NewTradingStatsManager(),
		logger:          logger.Named("dynamic-hedge"),
		stopChan:        make(chan struct{}),
		currentPhase:    "INITIALIZED",
	}

	// 初始化子管理器
	strategy.orderMonitor = NewOrderMonitor(
		strategy.orderManager,
		strategy.positionManager,
		lighterStrategy,
		binanceStrategy,
	)
	strategy.openingManager = NewOpeningManager(strategy)
	strategy.closingManager = NewClosingManager(strategy)
	strategy.hedgeBalancer = NewHedgeBalancer(strategy)
	strategy.fastExecutionManager = NewFastExecutionManager(strategy)

	return strategy
}

func NewPositionManager() *PositionManager {
	return &PositionManager{
		lighterPositions: &ExchangePositions{
			Exchange:  "lighter",
			Positions: make(map[string]*Position),
		},
		binancePositions: &ExchangePositions{
			Exchange:  "binance",
			Positions: make(map[string]*Position),
		},
		logger: logger.Named("position-manager"),
	}
}

func NewOrderManager() *OrderManager {
	return &OrderManager{
		activeOrders: make(map[string]*ActiveOrder),
		logger:       logger.Named("order-manager"),
	}
}

func NewRiskManager() *RiskManager {
	return &RiskManager{
		logger: logger.Named("risk-manager"),
	}
}

// Start 启动动态对冲策略
func (s *DynamicHedgeStrategy) Start(ctx context.Context, config *DynamicHedgeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("strategy is already running")
	}

	s.riskManager.config = config
	s.isRunning = true

	s.logger.Info("Starting dynamic hedge strategy",
		zap.Float64("order_size", config.OrderSize),
		zap.Float64("max_leverage", config.MaxLeverage),
		zap.Float64("emergency_leverage", config.EmergencyLeverage),
		zap.Duration("stop_duration", config.StopDuration),
	)

	// 配置快速执行
	if config.EnableFastExecution {
		fastConfig := &FastExecutionConfig{
			HighFrequencyMode:         true,
			CheckInterval:             config.FastCheckInterval,
			MaxExecutionDelay:         config.MaxExecutionDelay,
			EnablePreExecution:        config.EnablePreExecution,
			PartialFillThreshold:      config.PartialFillThreshold,
			EnablePriceProtection:     true,
			MaxSlippagePercent:        config.MaxSlippagePercent,
			PriceValidityWindow:       1 * time.Second,
			EnableConcurrentExecution: true,
			MaxConcurrentOrders:       3,
			EnableRetry:               true,
			MaxRetryAttempts:          3,
			RetryBackoffDuration:      100 * time.Millisecond,
		}
		s.fastExecutionManager.UpdateConfig(fastConfig)
		s.orderMonitor.SetFastExecutionManager(s.fastExecutionManager)
		s.orderMonitor.SetCheckInterval(config.FastCheckInterval)

		s.logger.Info("Fast execution enabled",
			zap.Duration("check_interval", config.FastCheckInterval),
			zap.Duration("max_delay", config.MaxExecutionDelay),
			zap.Bool("pre_execution", config.EnablePreExecution),
			zap.Float64("partial_threshold", config.PartialFillThreshold),
		)
	}

	// 启动订单监控
	if err := s.orderMonitor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start order monitor: %w", err)
	}

	// 启动主监控循环
	go s.monitoringLoop(ctx, config)

	return nil
}

// Stop 停止策略
func (s *DynamicHedgeStrategy) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	s.logger.Info("Stopping dynamic hedge strategy")

	// 停止订单监控
	s.orderMonitor.Stop()

	close(s.stopChan)
	s.isRunning = false
}

// monitoringLoop 主监控循环
func (s *DynamicHedgeStrategy) monitoringLoop(ctx context.Context, config *DynamicHedgeConfig) {
	ticker := time.NewTicker(config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context cancelled, stopping monitoring loop")
			return
		case <-s.stopChan:
			s.logger.Info("Stop signal received, stopping monitoring loop")
			return
		case <-ticker.C:
			if err := s.executeCycle(ctx, config); err != nil {
				s.logger.Error("Error in execution cycle", zap.Error(err))
			}
		}
	}
}

// executeCycle 执行一个周期的策略逻辑
func (s *DynamicHedgeStrategy) executeCycle(ctx context.Context, config *DynamicHedgeConfig) error {
	// 1. 更新统计信息
	s.updateStats(config)

	// 2. 检查日交易限制
	if config.ContinuousMode && s.shouldPauseForDay(config) {
		s.setPhase("DAILY_LIMIT_REACHED")
		s.logger.Info("Daily trading limit reached, pausing until next day")
		return nil
	}

	// 3. 更新仓位信息
	if err := s.updatePositions(ctx); err != nil {
		return fmt.Errorf("failed to update positions: %w", err)
	}

	// 4. 检查对冲平衡性
	if config.EnableHedgeBalancing {
		if err := s.checkAndAdjustHedgeBalance(ctx, config); err != nil {
			s.logger.Error("Failed to check hedge balance", zap.Error(err))
			// 不中断主流程，继续执行
		}
	}

	// 5. 检查风险状态
	riskStatus := s.riskManager.CheckRisk(s.positionManager)

	// 记录风险状态
	s.logger.Debug("Risk status check",
		zap.String("action", riskStatus.Action.String()),
		zap.Float64("max_leverage", riskStatus.MaxLeverage),
		zap.String("reason", riskStatus.Reason),
	)

	// 6. 根据风险状态执行相应逻辑
	switch riskStatus.Action {
	case RiskActionContinueOpening:
		return s.executeContinuousOpening(ctx, config)
	case RiskActionStopOpening:
		s.lastStopTime = time.Now()
		s.setPhase("LEVERAGE_LIMIT")
		s.logger.Warn("Stopping position opening due to leverage limit")
		return nil
	case RiskActionStartClosing:
		return s.executeContinuousClosing(ctx, config)
	case RiskActionEmergencyClose:
		s.setPhase("EMERGENCY_CLOSING")
		return s.closingManager.ExecuteEmergencyClosing(ctx, config)
	}

	return nil
}

// executeContinuousOpening 执行持续开仓
func (s *DynamicHedgeStrategy) executeContinuousOpening(ctx context.Context, config *DynamicHedgeConfig) error {
	// 检查是否可以进行新的交易
	if !s.canStartNewTrade(config) {
		return nil
	}

	s.setPhase("OPENING")
	s.logger.Info("Starting continuous opening phase")

	// 执行开仓逻辑
	err := s.openingManager.ExecuteOpeningLogic(ctx, config)
	if err != nil {
		s.logger.Error("Opening logic failed", zap.Error(err))
		return err
	}

	// 记录交易
	s.recordTrade(config.OrderSize, "OPENING")
	s.lastTradeTime = time.Now()

	return nil
}

// executeContinuousClosing 执行持续平仓
func (s *DynamicHedgeStrategy) executeContinuousClosing(ctx context.Context, config *DynamicHedgeConfig) error {
	s.setPhase("CLOSING")
	s.logger.Info("Starting continuous closing phase")

	// 执行平仓逻辑
	err := s.closingManager.ExecuteClosingLogic(ctx, config)
	if err != nil {
		s.logger.Error("Closing logic failed", zap.Error(err))
		return err
	}

	// 记录交易
	s.recordTrade(config.OrderSize, "CLOSING")
	s.lastTradeTime = time.Now()

	// 检查是否所有仓位已平仓，如果是则重新开始开仓
	if s.allPositionsZero() {
		s.setPhase("READY_FOR_OPENING")
		s.logger.Info("All positions closed, ready for new opening cycle")
	}

	return nil
}

// canStartNewTrade 检查是否可以开始新交易
func (s *DynamicHedgeStrategy) canStartNewTrade(config *DynamicHedgeConfig) bool {
	// 1. 检查交易间隔
	if !s.lastTradeTime.IsZero() && time.Since(s.lastTradeTime) < config.TradingInterval {
		return false
	}

	// 2. 检查是否有活跃订单
	activeOrders := s.orderManager.GetActiveOrders()
	if len(activeOrders) > 0 {
		s.logger.Debug("Has active orders, waiting for completion",
			zap.Int("active_orders", len(activeOrders)),
		)
		return false
	}

	// 3. 检查日交易次数限制
	if config.MaxDailyTrades > 0 && s.statsManager.ShouldPauseTradingForDay(config.MaxDailyTrades) {
		return false
	}

	return true
}

// shouldPauseForDay 检查是否应该暂停一天的交易
func (s *DynamicHedgeStrategy) shouldPauseForDay(config *DynamicHedgeConfig) bool {
	if !config.ContinuousMode {
		return false
	}

	// 检查日交易量目标
	volumeReached, tradesReached := s.statsManager.CheckDailyTargets(config.VolumeTarget, config.MaxDailyTrades)

	if volumeReached && tradesReached {
		return true
	}

	return false
}

// allPositionsZero 检查所有仓位是否为0
func (s *DynamicHedgeStrategy) allPositionsZero() bool {
	lighterPositions := s.positionManager.GetLighterPositions()
	binancePositions := s.positionManager.GetBinancePositions()

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

// setPhase 设置当前阶段
func (s *DynamicHedgeStrategy) setPhase(phase string) {
	s.mu.Lock()
	s.currentPhase = phase
	s.mu.Unlock()

	s.statsManager.UpdatePhase(phase)
}

// recordTrade 记录交易
func (s *DynamicHedgeStrategy) recordTrade(volume float64, tradeType string) {
	s.statsManager.RecordTrade(volume, tradeType)
}

// updateStats 更新统计信息
func (s *DynamicHedgeStrategy) updateStats(config *DynamicHedgeConfig) {
	// 更新活跃订单数
	activeOrders := s.orderManager.GetActiveOrders()
	s.statsManager.UpdateActiveOrders(len(activeOrders))

	// 更新交易量进度
	if config.VolumeTarget > 0 {
		s.statsManager.UpdateVolumeProgress(config.VolumeTarget)
	}

	// 定期输出统计日志 (每分钟一次)
	if time.Since(s.lastTradeTime) > time.Minute {
		s.statsManager.LogStats()
	}
}

// updatePositions 更新仓位信息
func (s *DynamicHedgeStrategy) updatePositions(ctx context.Context) error {
	// TODO: 实现从交易所获取实际仓位信息
	s.logger.Debug("Updating positions from exchanges")
	return nil
}

// GetStrategy 获取策略实例（供外部访问）
func (s *DynamicHedgeStrategy) GetStrategy() *DynamicHedgeStrategy {
	return s
}

// GetPositionSummary 获取仓位摘要
func (s *DynamicHedgeStrategy) GetPositionSummary() map[string]interface{} {
	return s.positionManager.GetPositionSummary()
}

// GetOrderSummary 获取订单摘要
func (s *DynamicHedgeStrategy) GetOrderSummary() map[string]*ActiveOrder {
	return s.orderManager.GetActiveOrders()
}

// IsRunning 检查策略是否运行中
func (s *DynamicHedgeStrategy) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetStats 获取交易统计信息
func (s *DynamicHedgeStrategy) GetStats() *TradingStats {
	if s.statsManager == nil {
		return nil
	}
	return s.statsManager.GetStats()
}

// checkAndAdjustHedgeBalance 检查并调整对冲平衡
func (s *DynamicHedgeStrategy) checkAndAdjustHedgeBalance(ctx context.Context, config *DynamicHedgeConfig) error {
	// 配置对冲平衡器参数
	if config.BalanceTolerance > 0 {
		s.hedgeBalancer.SetBalanceTolerance(config.BalanceTolerance)
	}
	if config.MinBalanceAdjust > 0 {
		s.hedgeBalancer.SetMinAdjustAmount(config.MinBalanceAdjust)
	}

	// 检查对冲平衡状态
	balanceStatus, err := s.hedgeBalancer.CheckHedgeBalance()
	if err != nil {
		return fmt.Errorf("failed to check hedge balance: %w", err)
	}

	s.logger.Debug("Hedge balance status",
		zap.Bool("is_balanced", balanceStatus.IsBalanced),
		zap.Int("imbalances_count", len(balanceStatus.Imbalances)),
		zap.Float64("total_imbalance_value", balanceStatus.TotalImbalanceValue),
	)

	// 如果存在不平衡且需要调整
	if !balanceStatus.IsBalanced && len(balanceStatus.Imbalances) > 0 {
		s.logger.Warn("Hedge imbalance detected, attempting to adjust",
			zap.Int("imbalances", len(balanceStatus.Imbalances)),
			zap.Float64("total_imbalance", balanceStatus.TotalImbalanceValue),
		)

		// 设置策略阶段为平衡调整
		s.setPhase("BALANCE_ADJUSTING")

		// 执行平衡调整
		if err := s.hedgeBalancer.ExecuteBalanceAdjustment(ctx, config, balanceStatus); err != nil {
			s.logger.Error("Failed to execute balance adjustment", zap.Error(err))
			return fmt.Errorf("failed to execute balance adjustment: %w", err)
		}

		s.logger.Info("Hedge balance adjustment completed successfully")
		s.setPhase("BALANCE_ADJUSTED")
	}

	return nil
}

// GetHedgeBalanceStatus 获取当前对冲平衡状态
func (s *DynamicHedgeStrategy) GetHedgeBalanceStatus() (*HedgeBalanceStatus, error) {
	return s.hedgeBalancer.CheckHedgeBalance()
}

// ForceBalanceAdjustment 强制执行平衡调整
func (s *DynamicHedgeStrategy) ForceBalanceAdjustment(ctx context.Context, config *DynamicHedgeConfig) error {
	s.logger.Info("Force balance adjustment requested")
	return s.checkAndAdjustHedgeBalance(ctx, config)
}

// GetExecutionStats 获取快速执行统计信息
func (s *DynamicHedgeStrategy) GetExecutionStats() *ExecutionStats {
	if s.fastExecutionManager == nil {
		return nil
	}
	return s.fastExecutionManager.GetExecutionStats()
}

// LogExecutionPerformance 记录执行性能指标
func (s *DynamicHedgeStrategy) LogExecutionPerformance() {
	if s.fastExecutionManager != nil {
		s.fastExecutionManager.LogPerformanceMetrics()
	}
}
