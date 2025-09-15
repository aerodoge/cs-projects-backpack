package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/logger"
)

// OrderMonitor 订单监控器
type OrderMonitor struct {
	orderManager         *OrderManager
	positionManager      *PositionManager
	lighterStrategy      *LighterStrategy
	binanceStrategy      *BinanceStrategy
	fastExecutionManager *FastExecutionManager
	logger               *zap.Logger

	// 监控状态
	isRunning bool
	stopChan  chan struct{}
	mu        sync.RWMutex

	// 配置
	checkInterval time.Duration
}

// OrderEvent 订单事件
type OrderEvent struct {
	Type      string       `json:"type"` // FILLED, PARTIAL_FILLED, CANCELLED
	Order     *ActiveOrder `json:"order"`
	Timestamp time.Time    `json:"timestamp"`
}

// NewOrderMonitor 创建订单监控器
func NewOrderMonitor(
	orderManager *OrderManager,
	positionManager *PositionManager,
	lighterStrategy *LighterStrategy,
	binanceStrategy *BinanceStrategy,
) *OrderMonitor {
	return &OrderMonitor{
		orderManager:    orderManager,
		positionManager: positionManager,
		lighterStrategy: lighterStrategy,
		binanceStrategy: binanceStrategy,
		logger:          logger.Named("order-monitor"),
		stopChan:        make(chan struct{}),
		checkInterval:   200 * time.Millisecond, // 默认高频检查
	}
}

// SetFastExecutionManager 设置快速执行管理器
func (om *OrderMonitor) SetFastExecutionManager(fem *FastExecutionManager) {
	om.fastExecutionManager = fem
}

// SetCheckInterval 设置检查间隔
func (om *OrderMonitor) SetCheckInterval(interval time.Duration) {
	om.checkInterval = interval
	om.logger.Info("Order monitor check interval updated",
		zap.Duration("interval", interval),
	)
}

// Start 启动订单监控
func (om *OrderMonitor) Start(ctx context.Context) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	if om.isRunning {
		return fmt.Errorf("order monitor is already running")
	}

	om.isRunning = true
	om.logger.Info("Starting order monitor")

	// 启动监控循环
	go om.monitorLoop(ctx)

	return nil
}

// Stop 停止订单监控
func (om *OrderMonitor) Stop() {
	om.mu.Lock()
	defer om.mu.Unlock()

	if !om.isRunning {
		return
	}

	om.logger.Info("Stopping order monitor")
	close(om.stopChan)
	om.isRunning = false
}

// monitorLoop 监控循环
func (om *OrderMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(om.checkInterval) // 使用可配置的检查间隔
	defer ticker.Stop()

	om.logger.Info("Order monitor loop started",
		zap.Duration("check_interval", om.checkInterval),
		zap.Bool("fast_execution_enabled", om.fastExecutionManager != nil),
	)

	for {
		select {
		case <-ctx.Done():
			om.logger.Info("Context cancelled, stopping order monitor")
			return
		case <-om.stopChan:
			om.logger.Info("Stop signal received, stopping order monitor")
			return
		case <-ticker.C:
			if err := om.checkActiveOrders(ctx); err != nil {
				om.logger.Error("Error checking active orders", zap.Error(err))
			}
		}
	}
}

// checkActiveOrders 检查活跃订单状态
func (om *OrderMonitor) checkActiveOrders(ctx context.Context) error {
	activeOrders := om.orderManager.GetActiveOrders()

	for _, order := range activeOrders {
		if err := om.checkOrderStatus(ctx, order); err != nil {
			om.logger.Error("Error checking order status",
				zap.String("order_id", order.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// checkOrderStatus 检查单个订单状态
func (om *OrderMonitor) checkOrderStatus(ctx context.Context, order *ActiveOrder) error {
	var newStatus string
	var filledSize float64
	var err error

	// 根据交易所查询订单状态
	switch order.Exchange {
	case "binance":
		newStatus, filledSize, err = om.getBinanceOrderStatus(ctx, order)
	case "lighter":
		newStatus, filledSize, err = om.getLighterOrderStatus(ctx, order)
	default:
		return fmt.Errorf("unknown exchange: %s", order.Exchange)
	}

	if err != nil {
		return fmt.Errorf("failed to get order status: %w", err)
	}

	// 检查状态是否有变化
	if newStatus != order.Status || filledSize != order.FilledSize {
		oldStatus := order.Status
		oldFilledSize := order.FilledSize

		// 更新订单状态
		om.orderManager.UpdateOrderStatus(order.ID, newStatus, filledSize)

		om.logger.Info("Order status updated",
			zap.String("order_id", order.ID),
			zap.String("old_status", oldStatus),
			zap.String("new_status", newStatus),
			zap.Float64("old_filled", oldFilledSize),
			zap.Float64("new_filled", filledSize),
		)

		// 处理订单状态变化
		if err := om.handleOrderStatusChange(ctx, order, oldStatus, newStatus); err != nil {
			return fmt.Errorf("failed to handle order status change: %w", err)
		}
	}

	return nil
}

// handleOrderStatusChange 处理订单状态变化
func (om *OrderMonitor) handleOrderStatusChange(ctx context.Context, order *ActiveOrder, oldStatus, newStatus string) error {
	switch newStatus {
	case "FILLED":
		return om.handleOrderFilled(ctx, order)
	case "PARTIAL":
		return om.handleOrderPartialFilled(ctx, order)
	case "CANCELLED":
		return om.handleOrderCancelled(ctx, order)
	}

	return nil
}

// handleOrderFilled 处理订单完全成交
func (om *OrderMonitor) handleOrderFilled(ctx context.Context, order *ActiveOrder) error {
	startTime := time.Now()

	om.logger.Info("Order fully filled, executing hedge trade",
		zap.String("order_id", order.ID),
		zap.String("exchange", order.Exchange),
		zap.String("symbol", order.Symbol),
		zap.String("side", order.Side),
		zap.Float64("size", order.Size),
	)

	// 使用快速执行管理器进行对冲交易
	if om.fastExecutionManager != nil {
		execCtx, err := om.fastExecutionManager.ExecuteFastHedge(
			ctx,
			order.ID,
			order.Symbol,
			order.Side,
			order.Size,
			order.Price,
		)

		if err != nil {
			om.logger.Error("Fast hedge execution failed",
				zap.String("order_id", order.ID),
				zap.Duration("total_delay", time.Since(startTime)),
				zap.Error(err),
			)
			return err
		}

		om.logger.Info("Fast hedge execution completed",
			zap.String("order_id", order.ID),
			zap.Duration("detection_to_execution", execCtx.TotalDelay),
			zap.Float64("execution_price", execCtx.ExecutionPrice),
			zap.Bool("success", execCtx.Success),
		)
	} else {
		// 降级到传统执行方式
		if err := om.executeHedgeTrade(ctx, order); err != nil {
			om.logger.Error("Failed to execute hedge trade",
				zap.String("order_id", order.ID),
				zap.Error(err),
			)
			return err
		}
	}

	// 更新仓位信息
	return om.updatePositionsAfterTrade(order)
}

// handleOrderPartialFilled 处理订单部分成交
func (om *OrderMonitor) handleOrderPartialFilled(ctx context.Context, order *ActiveOrder) error {
	om.logger.Info("Order partially filled, executing partial hedge",
		zap.String("order_id", order.ID),
		zap.Float64("filled_size", order.FilledSize),
		zap.Float64("remaining_size", order.Size-order.FilledSize),
	)

	// 计算新成交的部分
	newFilledSize := order.FilledSize // 这已经是更新后的总成交量

	// 为新成交部分执行对冲
	hedgeOrder := &ActiveOrder{
		Exchange: order.Exchange,
		Symbol:   order.Symbol,
		Side:     order.Side,
		Size:     newFilledSize, // 只对冲新成交的部分
	}

	if err := om.executeHedgeTrade(ctx, hedgeOrder); err != nil {
		om.logger.Error("Failed to execute partial hedge trade",
			zap.String("order_id", order.ID),
			zap.Error(err),
		)
		return err
	}

	// 更新仓位信息
	return om.updatePositionsAfterTrade(hedgeOrder)
}

// handleOrderCancelled 处理订单取消
func (om *OrderMonitor) handleOrderCancelled(ctx context.Context, order *ActiveOrder) error {
	om.logger.Warn("Order cancelled",
		zap.String("order_id", order.ID),
		zap.String("exchange", order.Exchange),
	)

	// 从活跃订单中移除
	om.orderManager.RemoveOrder(order.ID)

	return nil
}

// executeHedgeTrade 执行对冲交易
func (om *OrderMonitor) executeHedgeTrade(ctx context.Context, order *ActiveOrder) error {
	// 确定对冲方向和交易所
	var hedgeExchange string
	var hedgeSide string

	if order.Exchange == "binance" {
		hedgeExchange = "lighter"
		// Binance做空BTC -> Lighter做多BTC
		// Binance做多ETH -> Lighter做空ETH
		if order.Symbol == "BTC" && order.Side == "SELL" {
			hedgeSide = "BUY"
		} else if order.Symbol == "ETH" && order.Side == "BUY" {
			hedgeSide = "SELL"
		}
	} else {
		hedgeExchange = "binance"
		// Lighter做多BTC -> Binance做空BTC
		// Lighter做空ETH -> Binance做多ETH
		if order.Symbol == "BTC" && order.Side == "BUY" {
			hedgeSide = "SELL"
		} else if order.Symbol == "ETH" && order.Side == "SELL" {
			hedgeSide = "BUY"
		}
	}

	om.logger.Info("Executing hedge trade",
		zap.String("original_exchange", order.Exchange),
		zap.String("hedge_exchange", hedgeExchange),
		zap.String("symbol", order.Symbol),
		zap.String("hedge_side", hedgeSide),
		zap.Float64("size", order.Size),
	)

	// 执行对冲交易 (使用市价单快速成交)
	switch hedgeExchange {
	case "lighter":
		return om.executeLighterHedge(ctx, order.Symbol, hedgeSide, order.Size)
	case "binance":
		return om.executeBinanceHedge(ctx, order.Symbol, hedgeSide, order.Size)
	}

	return fmt.Errorf("unknown hedge exchange: %s", hedgeExchange)
}

// executeLighterHedge 在Lighter执行对冲
func (om *OrderMonitor) executeLighterHedge(ctx context.Context, symbol, side string, size float64) error {
	// TODO: 实现Lighter市价单对冲逻辑
	om.logger.Info("Executing Lighter hedge",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("size", size),
	)
	return nil
}

// executeBinanceHedge 在Binance执行对冲
func (om *OrderMonitor) executeBinanceHedge(ctx context.Context, symbol, side string, size float64) error {
	// TODO: 实现Binance市价单对冲逻辑
	om.logger.Info("Executing Binance hedge",
		zap.String("symbol", symbol),
		zap.String("side", side),
		zap.Float64("size", size),
	)
	return nil
}

// updatePositionsAfterTrade 交易后更新仓位
func (om *OrderMonitor) updatePositionsAfterTrade(order *ActiveOrder) error {
	// TODO: 实现仓位更新逻辑
	om.logger.Debug("Updating positions after trade",
		zap.String("symbol", order.Symbol),
		zap.Float64("size", order.Size),
	)
	return nil
}

// getBinanceOrderStatus 获取Binance订单状态
func (om *OrderMonitor) getBinanceOrderStatus(ctx context.Context, order *ActiveOrder) (string, float64, error) {
	// TODO: 实现Binance订单状态查询
	return "PENDING", 0, nil
}

// getLighterOrderStatus 获取Lighter订单状态
func (om *OrderMonitor) getLighterOrderStatus(ctx context.Context, order *ActiveOrder) (string, float64, error) {
	// TODO: 实现Lighter订单状态查询
	return "PENDING", 0, nil
}

// 订单管理器方法实现

// AddOrder 添加订单到监控
func (om *OrderManager) AddOrder(order *ActiveOrder) {
	om.mu.Lock()
	defer om.mu.Unlock()

	om.activeOrders[order.ID] = order
	om.logger.Info("Added order to monitoring",
		zap.String("order_id", order.ID),
		zap.String("exchange", order.Exchange),
		zap.String("symbol", order.Symbol),
	)
}

// GetActiveOrders 获取所有活跃订单
func (om *OrderManager) GetActiveOrders() map[string]*ActiveOrder {
	om.mu.RLock()
	defer om.mu.RUnlock()

	// 返回副本防止并发修改
	orders := make(map[string]*ActiveOrder)
	for id, order := range om.activeOrders {
		orders[id] = order
	}

	return orders
}

// UpdateOrderStatus 更新订单状态
func (om *OrderManager) UpdateOrderStatus(orderID, status string, filledSize float64) {
	om.mu.Lock()
	defer om.mu.Unlock()

	if order, exists := om.activeOrders[orderID]; exists {
		order.Status = status
		order.FilledSize = filledSize
		order.UpdatedAt = time.Now()

		// 如果订单完全成交或取消，从活跃列表中移除
		if status == "FILLED" || status == "CANCELLED" {
			delete(om.activeOrders, orderID)
		}
	}
}

// RemoveOrder 移除订单
func (om *OrderManager) RemoveOrder(orderID string) {
	om.mu.Lock()
	defer om.mu.Unlock()

	delete(om.activeOrders, orderID)
	om.logger.Debug("Removed order from monitoring", zap.String("order_id", orderID))
}
