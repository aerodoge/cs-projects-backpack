package strategy

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// HedgeBalancer 对冲平衡器 - 确保两个交易所的仓位保持对冲一致性
type HedgeBalancer struct {
	hedgeStrategy   *DynamicHedgeStrategy
	positionManager *PositionManager
	orderManager    *OrderManager
	logger          *zap.Logger

	// 平衡配置
	tolerancePercent float64 // 允许的仓位偏差百分比 (默认5%)
	minAdjustAmount  float64 // 最小调整金额 (避免微小调整)
}

// NewHedgeBalancer 创建对冲平衡器
func NewHedgeBalancer(hedgeStrategy *DynamicHedgeStrategy) *HedgeBalancer {
	return &HedgeBalancer{
		hedgeStrategy:    hedgeStrategy,
		positionManager:  hedgeStrategy.positionManager,
		orderManager:     hedgeStrategy.orderManager,
		logger:           hedgeStrategy.logger.Named("hedge-balancer"),
		tolerancePercent: 5.0,  // 5%容差
		minAdjustAmount:  50.0, // 最小50U调整
	}
}

// PositionImbalance 仓位不平衡信息
type PositionImbalance struct {
	Symbol           string  `json:"symbol"`            // BTC 或 ETH
	LighterPosition  float64 `json:"lighter_position"`  // Lighter仓位大小
	BinancePosition  float64 `json:"binance_position"`  // Binance仓位大小
	ExpectedBalance  float64 `json:"expected_balance"`  // 期望的平衡值
	ActualImbalance  float64 `json:"actual_imbalance"`  // 实际不平衡值
	ImbalancePercent float64 `json:"imbalance_percent"` // 不平衡百分比
	NeedsAdjustment  bool    `json:"needs_adjustment"`  // 是否需要调整
	AdjustmentSide   string  `json:"adjustment_side"`   // 调整方向 (LIGHTER_INCREASE, BINANCE_INCREASE)
	AdjustmentAmount float64 `json:"adjustment_amount"` // 调整金额
}

// CheckHedgeBalance 检查对冲平衡性
func (hb *HedgeBalancer) CheckHedgeBalance() (*HedgeBalanceStatus, error) {
	hb.logger.Debug("Checking hedge balance")

	lighterPositions := hb.positionManager.GetLighterPositions()
	binancePositions := hb.positionManager.GetBinancePositions()

	status := &HedgeBalanceStatus{
		IsBalanced:          true,
		Imbalances:          make([]*PositionImbalance, 0),
		CheckedAt:           time.Now(),
		TotalImbalanceValue: 0,
	}

	// 检查BTC仓位平衡
	btcImbalance := hb.checkSymbolBalance("BTC", lighterPositions, binancePositions)
	if btcImbalance.NeedsAdjustment {
		status.IsBalanced = false
		status.Imbalances = append(status.Imbalances, btcImbalance)
		status.TotalImbalanceValue += math.Abs(btcImbalance.AdjustmentAmount)
	}

	// 检查ETH仓位平衡
	ethImbalance := hb.checkSymbolBalance("ETH", lighterPositions, binancePositions)
	if ethImbalance.NeedsAdjustment {
		status.IsBalanced = false
		status.Imbalances = append(status.Imbalances, ethImbalance)
		status.TotalImbalanceValue += math.Abs(ethImbalance.AdjustmentAmount)
	}

	hb.logger.Info("Hedge balance check completed",
		zap.Bool("is_balanced", status.IsBalanced),
		zap.Int("imbalances_count", len(status.Imbalances)),
		zap.Float64("total_imbalance_value", status.TotalImbalanceValue),
	)

	return status, nil
}

// checkSymbolBalance 检查单个币种的仓位平衡
func (hb *HedgeBalancer) checkSymbolBalance(
	symbol string,
	lighterPositions, binancePositions *ExchangePositions,
) *PositionImbalance {
	// 获取仓位信息
	lighterPos := hb.getPositionValue(lighterPositions, symbol)
	binancePos := hb.getPositionValue(binancePositions, symbol)

	imbalance := &PositionImbalance{
		Symbol:          symbol,
		LighterPosition: lighterPos,
		BinancePosition: binancePos,
	}

	// 对冲策略：Lighter和Binance应该是相反的仓位
	// Lighter: BTC多头 + ETH空头
	// Binance: BTC空头 + ETH多头
	// 理想情况下：abs(lighter_position) = abs(binance_position)

	expectedBalance := (math.Abs(lighterPos) + math.Abs(binancePos)) / 2
	actualImbalance := math.Abs(lighterPos) - math.Abs(binancePos)

	imbalance.ExpectedBalance = expectedBalance
	imbalance.ActualImbalance = actualImbalance

	// 计算不平衡百分比
	if expectedBalance > 0 {
		imbalance.ImbalancePercent = math.Abs(actualImbalance) / expectedBalance * 100
	}

	// 判断是否需要调整
	imbalance.NeedsAdjustment = imbalance.ImbalancePercent > hb.tolerancePercent &&
		math.Abs(actualImbalance) > hb.minAdjustAmount

	if imbalance.NeedsAdjustment {
		// 确定调整方向和金额
		imbalance.AdjustmentAmount = math.Abs(actualImbalance) / 2 // 各调整一半

		if math.Abs(lighterPos) > math.Abs(binancePos) {
			// Lighter仓位过大，需要减少Lighter或增加Binance
			if symbol == "BTC" {
				// BTC: Lighter应该是多头，Binance应该是空头
				imbalance.AdjustmentSide = "BINANCE_INCREASE_SHORT"
			} else {
				// ETH: Lighter应该是空头，Binance应该是多头
				imbalance.AdjustmentSide = "BINANCE_INCREASE_LONG"
			}
		} else {
			// Binance仓位过大，需要减少Binance或增加Lighter
			if symbol == "BTC" {
				// BTC: 增加Lighter多头
				imbalance.AdjustmentSide = "LIGHTER_INCREASE_LONG"
			} else {
				// ETH: 增加Lighter空头
				imbalance.AdjustmentSide = "LIGHTER_INCREASE_SHORT"
			}
		}
	}

	hb.logger.Debug("Symbol balance check",
		zap.String("symbol", symbol),
		zap.Float64("lighter_position", lighterPos),
		zap.Float64("binance_position", binancePos),
		zap.Float64("expected_balance", expectedBalance),
		zap.Float64("actual_imbalance", actualImbalance),
		zap.Float64("imbalance_percent", imbalance.ImbalancePercent),
		zap.Bool("needs_adjustment", imbalance.NeedsAdjustment),
		zap.String("adjustment_side", imbalance.AdjustmentSide),
		zap.Float64("adjustment_amount", imbalance.AdjustmentAmount),
	)

	return imbalance
}

// getPositionValue 获取指定币种的仓位价值
func (hb *HedgeBalancer) getPositionValue(positions *ExchangePositions, symbol string) float64 {
	if pos, exists := positions.Positions[symbol]; exists {
		return pos.Value // 仓位价值（正数多头，负数空头）
	}
	return 0
}

// HedgeBalanceStatus 对冲平衡状态
type HedgeBalanceStatus struct {
	IsBalanced          bool                 `json:"is_balanced"`
	Imbalances          []*PositionImbalance `json:"imbalances"`
	TotalImbalanceValue float64              `json:"total_imbalance_value"`
	CheckedAt           time.Time            `json:"checked_at"`
	Recommendation      string               `json:"recommendation"`
}

// ExecuteBalanceAdjustment 执行平衡调整
func (hb *HedgeBalancer) ExecuteBalanceAdjustment(
	ctx context.Context,
	config *DynamicHedgeConfig,
	status *HedgeBalanceStatus,
) error {
	if status.IsBalanced {
		hb.logger.Info("Positions are already balanced, no adjustment needed")
		return nil
	}

	hb.logger.Info("Executing balance adjustment",
		zap.Int("imbalances_count", len(status.Imbalances)),
		zap.Float64("total_imbalance_value", status.TotalImbalanceValue),
	)

	for _, imbalance := range status.Imbalances {
		if err := hb.adjustSymbolBalance(ctx, config, imbalance); err != nil {
			hb.logger.Error("Failed to adjust symbol balance",
				zap.String("symbol", imbalance.Symbol),
				zap.Error(err),
			)
			return fmt.Errorf("failed to adjust %s balance: %w", imbalance.Symbol, err)
		}
	}

	hb.logger.Info("Balance adjustment completed successfully")
	return nil
}

// adjustSymbolBalance 调整单个币种的平衡
func (hb *HedgeBalancer) adjustSymbolBalance(
	ctx context.Context,
	config *DynamicHedgeConfig,
	imbalance *PositionImbalance,
) error {
	hb.logger.Info("Adjusting symbol balance",
		zap.String("symbol", imbalance.Symbol),
		zap.String("adjustment_side", imbalance.AdjustmentSide),
		zap.Float64("adjustment_amount", imbalance.AdjustmentAmount),
	)

	switch imbalance.AdjustmentSide {
	case "BINANCE_INCREASE_SHORT":
		return hb.increaseBinanceShort(ctx, imbalance.Symbol, imbalance.AdjustmentAmount, config)
	case "BINANCE_INCREASE_LONG":
		return hb.increaseBinanceLong(ctx, imbalance.Symbol, imbalance.AdjustmentAmount, config)
	case "LIGHTER_INCREASE_LONG":
		return hb.increaseLighterLong(ctx, imbalance.Symbol, imbalance.AdjustmentAmount, config)
	case "LIGHTER_INCREASE_SHORT":
		return hb.increaseLighterShort(ctx, imbalance.Symbol, imbalance.AdjustmentAmount, config)
	default:
		return fmt.Errorf("unknown adjustment side: %s", imbalance.AdjustmentSide)
	}
}

// increaseBinanceShort 增加Binance空头仓位
func (hb *HedgeBalancer) increaseBinanceShort(ctx context.Context, symbol string, amount float64, config *DynamicHedgeConfig) error {
	hb.logger.Info("Increasing Binance short position",
		zap.String("symbol", symbol),
		zap.Float64("amount", amount),
	)

	switch symbol {
	case "BTC":
		_, err := hb.hedgeStrategy.binanceStrategy.client.PlaceBTCShort(ctx, amount, config.SpreadPercent)
		return err
	case "ETH":
		return fmt.Errorf("ETH short not supported in this adjustment - ETH should be long on Binance")
	default:
		return fmt.Errorf("unsupported symbol for Binance short: %s", symbol)
	}
}

// increaseBinanceLong 增加Binance多头仓位
func (hb *HedgeBalancer) increaseBinanceLong(ctx context.Context, symbol string, amount float64, config *DynamicHedgeConfig) error {
	hb.logger.Info("Increasing Binance long position",
		zap.String("symbol", symbol),
		zap.Float64("amount", amount),
	)

	switch symbol {
	case "ETH":
		_, err := hb.hedgeStrategy.binanceStrategy.client.PlaceETHLong(ctx, amount, config.SpreadPercent)
		return err
	case "BTC":
		return fmt.Errorf("BTC long not supported in this adjustment - BTC should be short on Binance")
	default:
		return fmt.Errorf("unsupported symbol for Binance long: %s", symbol)
	}
}

// increaseLighterLong 增加Lighter多头仓位
func (hb *HedgeBalancer) increaseLighterLong(ctx context.Context, symbol string, amount float64, config *DynamicHedgeConfig) error {
	hb.logger.Info("Increasing Lighter long position",
		zap.String("symbol", symbol),
		zap.Float64("amount", amount),
	)

	usdtAmount := int64(amount)
	leverage := 3 // 固定3倍杠杆

	switch symbol {
	case "BTC":
		_, err := hb.hedgeStrategy.lighterStrategy.client.PlaceBTCLong(ctx, usdtAmount, leverage)
		return err
	case "ETH":
		return fmt.Errorf("ETH long not supported in this adjustment - ETH should be short on Lighter")
	default:
		return fmt.Errorf("unsupported symbol for Lighter long: %s", symbol)
	}
}

// increaseLighterShort 增加Lighter空头仓位
func (hb *HedgeBalancer) increaseLighterShort(ctx context.Context, symbol string, amount float64, config *DynamicHedgeConfig) error {
	hb.logger.Info("Increasing Lighter short position",
		zap.String("symbol", symbol),
		zap.Float64("amount", amount),
	)

	usdtAmount := int64(amount)
	leverage := 3 // 固定3倍杠杆

	switch symbol {
	case "ETH":
		_, err := hb.hedgeStrategy.lighterStrategy.client.PlaceETHShort(ctx, usdtAmount, leverage)
		return err
	case "BTC":
		return fmt.Errorf("BTC short not supported in this adjustment - BTC should be long on Lighter")
	default:
		return fmt.Errorf("unsupported symbol for Lighter short: %s", symbol)
	}
}

// GetBalanceRecommendation 获取平衡建议
func (hb *HedgeBalancer) GetBalanceRecommendation(status *HedgeBalanceStatus) string {
	if status.IsBalanced {
		return "Positions are well balanced. No action required."
	}

	var recommendations []string
	for _, imbalance := range status.Imbalances {
		rec := fmt.Sprintf("%s: %s (%.2f USDT adjustment)",
			imbalance.Symbol,
			imbalance.AdjustmentSide,
			imbalance.AdjustmentAmount)
		recommendations = append(recommendations, rec)
	}

	return fmt.Sprintf("Balance adjustments needed: %s", recommendations)
}

// SetBalanceTolerance 设置平衡容差
func (hb *HedgeBalancer) SetBalanceTolerance(tolerancePercent float64) {
	hb.tolerancePercent = tolerancePercent
	hb.logger.Info("Balance tolerance updated",
		zap.Float64("tolerance_percent", tolerancePercent),
	)
}

// SetMinAdjustAmount 设置最小调整金额
func (hb *HedgeBalancer) SetMinAdjustAmount(minAmount float64) {
	hb.minAdjustAmount = minAmount
	hb.logger.Info("Minimum adjustment amount updated",
		zap.Float64("min_adjust_amount", minAmount),
	)
}
