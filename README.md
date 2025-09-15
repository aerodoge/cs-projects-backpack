# 双交易所套利交易机器人

这个项目实现了基于Lighter和Binance交易所的双交易所套利策略，主要功能：

- **Lighter交易所** (作为Taker): 做多BTC + 做空ETH
- **Binance交易所** (作为Maker): 做空BTC + 做多ETH

## 功能特性

### 阶段一: Lighter交易所 (Taker策略)
- **BTC做多**: 市价单买入BTC，1000 USDT本位，3倍杠杆
- **ETH做空**: 市价单卖出ETH，1000 USDT本位，3倍杠杆

### 阶段二: Binance交易所 (Maker策略)
- **BTC做空**: 限价单卖出BTC，1000 USDT，0.1%价差
- **ETH做多**: 限价单买入ETH，1000 USDT，0.1%价差

## 使用方法

### 配置方式

项目支持多种配置方式，按优先级从高到低：

1. **环境变量** (最高优先级)
2. **YAML配置文件**
3. **默认值** (最低优先级)

#### 方式1: 使用YAML配置文件 (推荐)

```bash
# 复制配置模板
cp configs/config.yaml.example config.yaml

# 编辑配置文件
vim config.yaml
```

#### 配置项说明

**Lighter交易所配置 (必填):**
- `lighter.api_key`: Lighter API密钥
- `lighter.secret_key`: Lighter Secret密钥
- `lighter.private_key`: 十六进制格式的私钥 (80个字符，40字节)

**Binance交易所配置 (必填):**
- `binance.api_key`: Binance API密钥
- `binance.secret_key`: Binance Secret密钥
- `binance.testnet`: 是否使用测试网 (默认: false)

**可选配置(有默认值):**
- `lighter.base_url`: API地址 (默认: https://api.lighter.xyz)
- `lighter.chain_id`: 链ID (默认: 1)
- `lighter.account_index`: 账户索引 (默认: 1)
- `lighter.api_key_index`: API密钥索引 (默认: 0)
- `trading.usdt_amount`: 每次交易USDT数量 (默认: 1000)
- `trading.leverage`: Lighter杠杆倍数 (默认: 3)
- `logging.level`: 日志级别 (默认: info)

### 编译和运行

#### 使用Makefile (推荐)

```bash
# 查看所有可用命令
make help

# 编译程序 (会自动整理依赖)
make build

# 前台运行
make run

# 后台运行
make daemon

# 查看运行状态
make status

# 查看实时日志
make logs

# 停止程序
make stop

# 重启程序
make restart
```

#### 手动编译

```bash
# 安装依赖
go mod tidy

# 编译
go build -o build/lighter-trader cmd/main.go

# 运行
./build/lighter-trader
```

## 配置说明

### 套利交易规格
- **交易方式**: USDT本位合约
- **固定金额**: 每次1000 USDT
- **Lighter杠杆**: 3倍杠杆
- **Binance价差**: 0.1% (确保作为Maker成交)

### Lighter交易所配置
- **订单类型**: 市价单 (作为Taker)
- **BTC市场索引**: 0
- **ETH市场索引**: 1
- **订单方向**: IsAsk = 0 (买入), IsAsk = 1 (卖出)

### Binance交易所配置
- **订单类型**: 限价单 (作为Maker)
- **交易对**: BTCUSDT, ETHUSDT
- **价格策略**: 基于当前市价±0.1%设置限价

## Makefile命令参考

### 构建和运行
- `make help` - 显示所有可用命令
- `make deps` - 整理Go依赖
- `make build` - 编译程序
- `make run` - 前台运行程序
- `make daemon` - 后台运行程序
- `make stop` - 停止后台程序
- `make restart` - 重启程序
- `make status` - 查看程序运行状态
- `make logs` - 查看实时日志

### 代码维护
- `make clean` - 清理编译文件
- `make test` - 运行测试
- `make fmt` - 格式化代码
- `make lint` - 代码静态检查

### 开发工具
- `make dev-deps` - 安装开发依赖工具
- `make ci` - 运行完整CI检查

### 签名机制
使用Poseidon哈希和Schnorr签名来确保交易安全性。

### 套利交易流程
1. **初始化阶段**
   - 加载配置 (支持多种配置源)
   - 初始化日志系统
   - 创建Lighter和Binance客户端
   - 验证API连接

2. **阶段一: Lighter交易所执行 (Taker)**
   - 下BTC市价多单 (1000 USDT, 3x杠杆)
   - 下ETH市价空单 (1000 USDT, 3x杠杆)
   - 记录交易哈希

3. **阶段二: Binance交易所执行 (Maker)**
   - 获取当前BTC/ETH价格
   - 计算最优Maker价格 (±0.1%价差)
   - 下BTC限价空单 (1000 USDT)
   - 下ETH限价多单 (1000 USDT)
   - 记录订单ID

4. **完成总结**
   - 输出所有头寸信息
   - 记录套利策略执行结果

## 注意事项

1. **安全性**: 请妥善保管你的私钥和API密钥
2. **网络**: 确保网络连接稳定
3. **余额**: 确保两个交易所账户都有足够余额
4. **测试**: 建议先在测试环境中验证功能
5. **风险**: 套利策略存在市场风险，请谨慎使用
6. **时间**: 两个交易所订单执行有时间差，注意市场波动风险

## 依赖项

### 核心依赖
- `github.com/elliottech/lighter-go`: Lighter Go SDK
- `github.com/adshao/go-binance/v2`: Binance Go SDK
- `github.com/spf13/viper`: 配置管理库
- `go.uber.org/zap`: 高性能日志库
- `gopkg.in/natefinch/lumberjack.v2`: 日志轮转库

### 间接依赖
- `github.com/elliottech/poseidon_crypto`: Poseidon加密库
- `github.com/ethereum/go-ethereum`: 以太坊相关工具
- `github.com/fsnotify/fsnotify`: 文件系统监控
- `go.uber.org/multierr`: 多错误处理
- `github.com/gorilla/websocket`: WebSocket连接 (Binance)

## 许可证

MIT License