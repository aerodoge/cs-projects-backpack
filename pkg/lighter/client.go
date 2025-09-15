package lighter

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"go.uber.org/zap"

	"cs-projects-backpack/pkg/config"
	"cs-projects-backpack/pkg/logger"

	"github.com/elliottech/lighter-go/signer"
	"github.com/elliottech/lighter-go/types"
	"github.com/elliottech/lighter-go/types/txtypes"
)

type Client struct {
	signer       signer.Signer
	config       *config.LighterConfig
	chainId      uint32
	accountIndex int64
	apiKeyIndex  uint8
	logger       *zap.Logger
}

type MarketOrderRequest struct {
	MarketIndex uint8
	USDTAmount  int64 // USDT数量
	Leverage    int   // 杠杆倍数
	IsAsk       uint8 // 0=买入(做多), 1=卖出(做空)
}

const (
	BTCMarketIndex uint8 = 0
	ETHMarketIndex uint8 = 1
)

func NewClient(cfg *config.LighterConfig) (*Client, error) {
	log := logger.Named("lighter-client")

	if cfg.APIKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("API key and secret key are required")
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("private key is required")
	}

	// 将十六进制私钥转换为字节数组
	privateKeyBytes, err := hex.DecodeString(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key hex: %w", err)
	}

	if len(privateKeyBytes) != 40 {
		return nil, fmt.Errorf("invalid private key length: expected 40 bytes, got %d", len(privateKeyBytes))
	}

	signerInstance, err := signer.NewKeyManager(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	log.Info("Lighter client initialized",
		zap.String("base_url", cfg.BaseURL),
		zap.Uint32("chain_id", cfg.ChainID),
		zap.Int64("account_index", cfg.AccountIndex),
		zap.Uint8("api_key_index", cfg.APIKeyIndex),
	)

	return &Client{
		signer:       signerInstance,
		config:       cfg,
		chainId:      cfg.ChainID,
		accountIndex: cfg.AccountIndex,
		apiKeyIndex:  cfg.APIKeyIndex,
		logger:       log,
	}, nil
}

func (c *Client) createOrderTransaction(req *MarketOrderRequest) (*txtypes.L2CreateOrderTxInfo, error) {
	now := time.Now()
	nonce := now.UnixMilli()
	expiredAt := now.Add(30 * time.Minute).UnixMilli()

	// 计算基础资产数量 (USDT * 杠杆倍数)
	// 注意：这里的计算可能需要根据Lighter的实际单位进行调整
	leveragedAmount := req.USDTAmount * int64(req.Leverage)

	c.logger.Debug("Creating order transaction",
		zap.Uint8("market_index", req.MarketIndex),
		zap.Int64("usdt_amount", req.USDTAmount),
		zap.Int("leverage", req.Leverage),
		zap.Int64("leveraged_amount", leveragedAmount),
		zap.Uint8("is_ask", req.IsAsk),
	)

	createOrderReq := &types.CreateOrderTxReq{
		MarketIndex:      req.MarketIndex,
		ClientOrderIndex: nonce,
		BaseAmount:       leveragedAmount,       // 使用杠杆后的数量
		Price:            txtypes.NilOrderPrice, // 市价单无需指定价格
		IsAsk:            req.IsAsk,
		Type:             txtypes.MarketOrder,
		TimeInForce:      txtypes.ImmediateOrCancel,
		ReduceOnly:       0, // 开仓订单
		TriggerPrice:     txtypes.NilOrderTriggerPrice,
		OrderExpiry:      txtypes.NilOrderExpiry,
	}

	transactOpts := &types.TransactOpts{
		FromAccountIndex: &c.accountIndex,
		ApiKeyIndex:      &c.apiKeyIndex,
		ExpiredAt:        expiredAt,
		Nonce:            &nonce,
		DryRun:           false,
	}

	return types.ConstructCreateOrderTx(c.signer, c.chainId, createOrderReq, transactOpts)
}

func (c *Client) PlaceMarketOrder(ctx context.Context, req *MarketOrderRequest) (*txtypes.L2CreateOrderTxInfo, error) {
	c.logger.Info("Creating market order",
		zap.Uint8("market_index", req.MarketIndex),
		zap.Int64("usdt_amount", req.USDTAmount),
		zap.Int("leverage", req.Leverage),
		zap.Uint8("is_ask", req.IsAsk),
	)

	orderTx, err := c.createOrderTransaction(req)
	if err != nil {
		c.logger.Error("Failed to create order transaction",
			zap.Error(err),
			zap.Uint8("market_index", req.MarketIndex),
		)
		return nil, fmt.Errorf("failed to create order transaction: %w", err)
	}

	c.logger.Info("Market order created successfully",
		zap.String("tx_hash", orderTx.GetTxHash()),
		zap.Uint8("market_index", req.MarketIndex),
		zap.Int64("usdt_amount", req.USDTAmount),
		zap.Int("leverage", req.Leverage),
	)

	return orderTx, nil
}

func (c *Client) PlaceBTCLong(ctx context.Context, usdtAmount int64, leverage int) (*txtypes.L2CreateOrderTxInfo, error) {
	c.logger.Info("Placing BTC long order",
		zap.Int64("usdt_amount", usdtAmount),
		zap.Int("leverage", leverage),
	)

	req := &MarketOrderRequest{
		MarketIndex: BTCMarketIndex,
		USDTAmount:  usdtAmount,
		Leverage:    leverage,
		IsAsk:       0, // 0 = 买入(做多)
	}

	return c.PlaceMarketOrder(ctx, req)
}

func (c *Client) PlaceETHShort(ctx context.Context, usdtAmount int64, leverage int) (*txtypes.L2CreateOrderTxInfo, error) {
	c.logger.Info("Placing ETH short order",
		zap.Int64("usdt_amount", usdtAmount),
		zap.Int("leverage", leverage),
	)

	req := &MarketOrderRequest{
		MarketIndex: ETHMarketIndex,
		USDTAmount:  usdtAmount,
		Leverage:    leverage,
		IsAsk:       1, // 1 = 卖出(做空)
	}

	return c.PlaceMarketOrder(ctx, req)
}
