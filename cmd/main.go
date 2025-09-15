package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/binance"
	"cs-projects-backpack/pkg/config"
	"cs-projects-backpack/pkg/lighter"
	"cs-projects-backpack/pkg/logger"
	"cs-projects-backpack/pkg/strategy"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.Initialize(&cfg.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	log.Info("Starting Trading Bot",
		zap.String("app_name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("environment", cfg.App.Environment),
		zap.String("strategy_type", cfg.Strategy.Type),
	)

	if err := cfg.Validate(); err != nil {
		log.Fatal("Configuration validation failed", zap.Error(err))
	}

	log.Info("Configuration loaded successfully")

	// 创建可取消的上下文和信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("Received shutdown signal", zap.String("signal", sig.String()))
		log.Info("Initiating graceful shutdown...")
		cancel()
	}()

	switch cfg.Strategy.Type {
	case "lighter":
		err = runLighterStrategy(ctx, cfg, log)
	case "binance":
		err = runBinanceStrategy(ctx, cfg, log)
	case "arbitrage":
		err = runArbitrageStrategy(ctx, cfg, log)
	case "dynamic_hedge":
		err = runDynamicHedgeStrategy(ctx, cfg, log)
	default:
		log.Fatal("Unknown strategy type", zap.String("type", cfg.Strategy.Type))
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			log.Info("Strategy stopped due to shutdown signal")
		} else {
			log.Fatal("Strategy execution failed", zap.Error(err))
		}
	} else {
		log.Info("Strategy execution completed successfully")
	}
}

func runLighterStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Lighter Strategy ===")

	lighterClient, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		return fmt.Errorf("failed to create Lighter client: %w", err)
	}

	lighterStrategy := strategy.NewLighterStrategy(lighterClient)

	lighterConfig := &strategy.LighterConfig{
		USDTAmount: cfg.Trading.USDTAmount,
		Leverage:   cfg.Trading.Leverage,
	}

	log.Info("Press Ctrl+C to stop the strategy...")

	errChan := make(chan error, 1)
	go func() {
		errChan <- lighterStrategy.ExecuteBTCETHPair(ctx, lighterConfig)
	}()

	select {
	case <-ctx.Done():
		log.Info("Lighter strategy stopped due to shutdown signal")
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func runBinanceStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Binance Strategy ===")

	binanceClient, err := binance.NewClient(&cfg.Binance)
	if err != nil {
		return fmt.Errorf("failed to create Binance client: %w", err)
	}

	binanceStrategy := strategy.NewBinanceStrategy(binanceClient)

	binanceConfig := &strategy.BinanceConfig{
		USDCAmount:    float64(cfg.Trading.USDCAmount),
		SpreadPercent: cfg.Strategy.SpreadPercent,
	}

	log.Info("Press Ctrl+C to stop the strategy...")

	errChan := make(chan error, 1)
	go func() {
		errChan <- binanceStrategy.ExecuteBTCETHPair(ctx, binanceConfig)
	}()

	select {
	case <-ctx.Done():
		log.Info("Binance strategy stopped due to shutdown signal")
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func runArbitrageStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Arbitrage Strategy ===")

	// Create Lighter client
	lighterClient, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		return fmt.Errorf("failed to create Lighter client: %w", err)
	}

	// Create Binance client
	binanceClient, err := binance.NewClient(&cfg.Binance)
	if err != nil {
		return fmt.Errorf("failed to create Binance client: %w", err)
	}

	// Create individual strategies
	lighterStrategy := strategy.NewLighterStrategy(lighterClient)
	binanceStrategy := strategy.NewBinanceStrategy(binanceClient)

	// Create arbitrage strategy
	arbitrageStrategy := strategy.NewArbitrageStrategy(lighterStrategy, binanceStrategy)

	arbitrageConfig := &strategy.ArbitrageConfig{
		USDTAmount:    cfg.Trading.USDTAmount,
		USDCAmount:    cfg.Trading.USDCAmount,
		Leverage:      cfg.Trading.Leverage,
		SpreadPercent: cfg.Strategy.SpreadPercent,
	}

	log.Info("Press Ctrl+C to stop the strategy...")

	errChan := make(chan error, 1)
	go func() {
		errChan <- arbitrageStrategy.ExecuteBTCETHArbitrage(ctx, arbitrageConfig)
	}()

	select {
	case <-ctx.Done():
		log.Info("Arbitrage strategy stopped due to shutdown signal")
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func runDynamicHedgeStrategy(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	log.Info("=== Running Dynamic Hedge Strategy ===")

	// Create Lighter client
	lighterClient, err := lighter.NewClient(&cfg.Lighter)
	if err != nil {
		return fmt.Errorf("failed to create Lighter client: %w", err)
	}

	// Create Binance client
	binanceClient, err := binance.NewClient(&cfg.Binance)
	if err != nil {
		return fmt.Errorf("failed to create Binance client: %w", err)
	}

	// Create individual strategies
	lighterStrategy := strategy.NewLighterStrategy(lighterClient)
	binanceStrategy := strategy.NewBinanceStrategy(binanceClient)

	// Create dynamic hedge strategy
	dynamicHedgeStrategy := strategy.NewDynamicHedgeStrategy(lighterStrategy, binanceStrategy)

	// Configure dynamic hedge parameters
	dynamicConfig := &strategy.DynamicHedgeConfig{
		OrderSize:         float64(cfg.Trading.USDCAmount), // 使用USDC作为基准
		MaxLeverage:       cfg.Strategy.MaxLeverage,
		EmergencyLeverage: cfg.Strategy.EmergencyLeverage,
		StopDuration:      cfg.Strategy.StopDuration,
		MonitorInterval:   cfg.Strategy.MonitorInterval,
		SpreadPercent:     cfg.Strategy.SpreadPercent,

		// 持续交易配置
		ContinuousMode:  cfg.Strategy.ContinuousMode,
		TradingInterval: cfg.Strategy.TradingInterval,
		VolumeTarget:    cfg.Strategy.VolumeTarget,
		MaxDailyTrades:  cfg.Strategy.MaxDailyTrades,

		// 对冲平衡配置
		EnableHedgeBalancing: cfg.Strategy.EnableHedgeBalancing,
		BalanceCheckInterval: cfg.Strategy.BalanceCheckInterval,
		BalanceTolerance:     cfg.Strategy.BalanceTolerance,
		MinBalanceAdjust:     cfg.Strategy.MinBalanceAdjust,

		// 快速执行配置
		EnableFastExecution:  cfg.Strategy.EnableFastExecution,
		FastCheckInterval:    cfg.Strategy.FastCheckInterval,
		MaxExecutionDelay:    cfg.Strategy.MaxExecutionDelay,
		EnablePreExecution:   cfg.Strategy.EnablePreExecution,
		PartialFillThreshold: cfg.Strategy.PartialFillThreshold,
		MaxSlippagePercent:   cfg.Strategy.MaxSlippagePercent,
	}

	log.Info("Starting dynamic hedge strategy with config",
		zap.Float64("order_size", dynamicConfig.OrderSize),
		zap.Float64("max_leverage", dynamicConfig.MaxLeverage),
		zap.Float64("emergency_leverage", dynamicConfig.EmergencyLeverage),
		zap.Duration("stop_duration", dynamicConfig.StopDuration),
		zap.Duration("monitor_interval", dynamicConfig.MonitorInterval),
		zap.Bool("continuous_mode", dynamicConfig.ContinuousMode),
		zap.Duration("trading_interval", dynamicConfig.TradingInterval),
		zap.Float64("volume_target", dynamicConfig.VolumeTarget),
		zap.Int("max_daily_trades", dynamicConfig.MaxDailyTrades),
		zap.Bool("enable_hedge_balancing", dynamicConfig.EnableHedgeBalancing),
		zap.Duration("balance_check_interval", dynamicConfig.BalanceCheckInterval),
		zap.Float64("balance_tolerance", dynamicConfig.BalanceTolerance),
		zap.Float64("min_balance_adjust", dynamicConfig.MinBalanceAdjust),
		zap.Bool("enable_fast_execution", dynamicConfig.EnableFastExecution),
		zap.Duration("fast_check_interval", dynamicConfig.FastCheckInterval),
		zap.Duration("max_execution_delay", dynamicConfig.MaxExecutionDelay),
		zap.Bool("enable_pre_execution", dynamicConfig.EnablePreExecution),
		zap.Float64("partial_fill_threshold", dynamicConfig.PartialFillThreshold),
		zap.Float64("max_slippage_percent", dynamicConfig.MaxSlippagePercent),
	)

	// Start the dynamic hedge strategy
	if err := dynamicHedgeStrategy.Start(ctx, dynamicConfig); err != nil {
		return fmt.Errorf("failed to start dynamic hedge strategy: %w", err)
	}

	log.Info("Dynamic hedge strategy started successfully")
	log.Info("Press Ctrl+C to stop the strategy gracefully...")

	// Wait for context cancellation (Ctrl+C)
	<-ctx.Done()

	log.Info("Shutdown signal received, stopping dynamic hedge strategy...")

	// 获取最终统计信息
	if stats := dynamicHedgeStrategy.GetStats(); stats != nil {
		log.Info("Final trading statistics",
			zap.Float64("daily_volume", stats.DailyVolume),
			zap.Int("daily_trades", stats.DailyTrades),
			zap.Float64("total_volume", stats.TotalVolume),
			zap.Int("total_trades", stats.TotalTrades),
		)
	}

	// 获取执行性能统计
	if execStats := dynamicHedgeStrategy.GetExecutionStats(); execStats != nil {
		log.Info("Final execution performance statistics",
			zap.Int64("total_executions", execStats.TotalExecutions),
			zap.Int64("successful_executions", execStats.SuccessfulExecutions),
			zap.Float64("success_rate", float64(execStats.SuccessfulExecutions)/float64(execStats.TotalExecutions)*100),
			zap.Duration("average_delay", execStats.AverageDelay),
			zap.Duration("min_delay", execStats.MinDelay),
			zap.Duration("max_delay", execStats.MaxDelay),
			zap.Any("delay_distribution", execStats.DelayBuckets),
		)
	}

	// 停止
	dynamicHedgeStrategy.Stop()
	log.Info("Dynamic hedge strategy stopped successfully")

	return ctx.Err()
}
