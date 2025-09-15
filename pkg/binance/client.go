package binance

import (
	"context"
	"fmt"
	"strconv"

	"github.com/adshao/go-binance/v2"
	"go.uber.org/zap"

	"cs-projects-backpack/pkg/config"
	"cs-projects-backpack/pkg/logger"
)

type Client struct {
	client *binance.Client
	config *config.BinanceConfig
	logger *zap.Logger
}

type OrderRequest struct {
	Symbol   string
	Side     binance.SideType
	Quantity string
	Price    string // 限价单价格，空字符串表示市价单
}

const (
	BTCUSDCSymbol = "BTCUSDC"
	ETHUSDCSymbol = "ETHUSDC"
)

func NewClient(cfg *config.BinanceConfig) (*Client, error) {
	log := logger.Named("binance-client")

	if cfg.APIKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("binance API key and secret key are required")
	}

	// 设置测试网络
	if cfg.Testnet {
		binance.UseTestnet = true
		log.Info("Using Binance testnet")
	}

	client := binance.NewClient(cfg.APIKey, cfg.SecretKey)

	log.Info("Binance client initialized",
		zap.Bool("testnet", cfg.Testnet),
	)

	return &Client{
		client: client,
		config: cfg,
		logger: log,
	}, nil
}

// PlaceLimitOrder 下限价单 (作为Maker)
func (c *Client) PlaceLimitOrder(ctx context.Context, req *OrderRequest) (*binance.CreateOrderResponse, error) {
	c.logger.Info("Placing limit order",
		zap.String("symbol", req.Symbol),
		zap.String("side", string(req.Side)),
		zap.String("quantity", req.Quantity),
		zap.String("price", req.Price),
	)

	order, err := c.client.NewCreateOrderService().
		Symbol(req.Symbol).
		Side(req.Side).
		Type(binance.OrderTypeLimit).
		TimeInForce(binance.TimeInForceTypeGTC). // Good Till Cancelled
		Quantity(req.Quantity).
		Price(req.Price).
		Do(ctx)

	if err != nil {
		c.logger.Error("Failed to place limit order",
			zap.Error(err),
			zap.String("symbol", req.Symbol),
		)
		return nil, fmt.Errorf("failed to place limit order: %w", err)
	}

	c.logger.Info("Limit order placed successfully",
		zap.Int64("order_id", order.OrderID),
		zap.String("symbol", req.Symbol),
		zap.String("side", string(req.Side)),
		zap.String("quantity", req.Quantity),
		zap.String("price", req.Price),
	)

	return order, nil
}

// GetCurrentPrice 获取当前价格
func (c *Client) GetCurrentPrice(ctx context.Context, symbol string) (float64, error) {
	ticker, err := c.client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get price for %s: %w", symbol, err)
	}

	if len(ticker) == 0 {
		return 0, fmt.Errorf("no price data for %s", symbol)
	}

	price, err := strconv.ParseFloat(ticker[0].Price, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price: %w", err)
	}

	return price, nil
}

// CalculateQuantityFromUSDC 根据USDC数量计算对应的币种数量
func (c *Client) CalculateQuantityFromUSDC(ctx context.Context, symbol string, usdcAmount float64) (string, error) {
	price, err := c.GetCurrentPrice(ctx, symbol)
	if err != nil {
		return "", err
	}

	quantity := usdcAmount / price

	// 根据币种调整精度
	var precision int
	switch symbol {
	case BTCUSDCSymbol:
		precision = 6 // BTC通常保留6位小数
	case ETHUSDCSymbol:
		precision = 5 // ETH通常保留5位小数
	default:
		precision = 4 // 默认4位小数
	}

	quantityStr := fmt.Sprintf("%."+strconv.Itoa(precision)+"f", quantity)

	c.logger.Debug("Calculated quantity",
		zap.String("symbol", symbol),
		zap.Float64("price", price),
		zap.Float64("usdc_amount", usdcAmount),
		zap.String("quantity", quantityStr),
	)

	return quantityStr, nil
}

// GetOptimalPrice 获取最优挂单价格 (作为Maker)
func (c *Client) GetOptimalPrice(ctx context.Context, symbol string, side binance.SideType, spreadPercent float64) (string, error) {
	currentPrice, err := c.GetCurrentPrice(ctx, symbol)
	if err != nil {
		return "", err
	}

	var optimalPrice float64
	if side == binance.SideTypeBuy {
		// 买单：当前价格 * (1 - spread)，确保作为Maker
		optimalPrice = currentPrice * (1 - spreadPercent/100)
	} else {
		// 卖单：当前价格 * (1 + spread)，确保作为Maker
		optimalPrice = currentPrice * (1 + spreadPercent/100)
	}

	// 价格精度处理
	var pricePrecision int
	switch symbol {
	case BTCUSDCSymbol:
		pricePrecision = 2 // BTC/USDC 价格保留2位小数
	case ETHUSDCSymbol:
		pricePrecision = 2 // ETH/USDC 价格保留2位小数
	default:
		pricePrecision = 4 // 默认4位小数
	}

	priceStr := fmt.Sprintf("%."+strconv.Itoa(pricePrecision)+"f", optimalPrice)

	c.logger.Debug("Calculated optimal price",
		zap.String("symbol", symbol),
		zap.String("side", string(side)),
		zap.Float64("current_price", currentPrice),
		zap.Float64("spread_percent", spreadPercent),
		zap.String("optimal_price", priceStr),
	)

	return priceStr, nil
}

// PlaceBTCShort 做空BTC (卖出BTC)
func (c *Client) PlaceBTCShort(ctx context.Context, usdcAmount float64, spreadPercent float64) (*binance.CreateOrderResponse, error) {
	c.logger.Info("Placing BTC short order",
		zap.Float64("usdc_amount", usdcAmount),
		zap.Float64("spread_percent", spreadPercent),
	)

	// 计算数量
	quantity, err := c.CalculateQuantityFromUSDC(ctx, BTCUSDCSymbol, usdcAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate BTC quantity: %w", err)
	}

	// 获取最优价格 (作为Maker)
	price, err := c.GetOptimalPrice(ctx, BTCUSDCSymbol, binance.SideTypeSell, spreadPercent)
	if err != nil {
		return nil, fmt.Errorf("failed to get optimal price: %w", err)
	}

	req := &OrderRequest{
		Symbol:   BTCUSDCSymbol,
		Side:     binance.SideTypeSell,
		Quantity: quantity,
		Price:    price,
	}

	return c.PlaceLimitOrder(ctx, req)
}

// PlaceETHLong 做多ETH (买入ETH)
func (c *Client) PlaceETHLong(ctx context.Context, usdcAmount float64, spreadPercent float64) (*binance.CreateOrderResponse, error) {
	c.logger.Info("Placing ETH long order",
		zap.Float64("usdc_amount", usdcAmount),
		zap.Float64("spread_percent", spreadPercent),
	)

	// 计算数量
	quantity, err := c.CalculateQuantityFromUSDC(ctx, ETHUSDCSymbol, usdcAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate ETH quantity: %w", err)
	}

	// 获取最优价格 (作为Maker)
	price, err := c.GetOptimalPrice(ctx, ETHUSDCSymbol, binance.SideTypeBuy, spreadPercent)
	if err != nil {
		return nil, fmt.Errorf("failed to get optimal price: %w", err)
	}

	req := &OrderRequest{
		Symbol:   ETHUSDCSymbol,
		Side:     binance.SideTypeBuy,
		Quantity: quantity,
		Price:    price,
	}

	return c.PlaceLimitOrder(ctx, req)
}
