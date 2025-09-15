# Lighter Exchange Trading Bot

这个项目实现了基于Lighter交易所Go SDK的自动化交易功能，主要实现：

- 在Lighter交易所做多BTC
- 在Lighter交易所做空ETH

## 功能特性

### 1. BTC市价多单
- 使用市价单在Lighter交易所买入BTC
- 固定1000 USDT本位交易
- 3倍杠杆
- 作为Taker执行

### 2. ETH市价空单
- 使用市价单在Lighter交易所卖出ETH
- 固定1000 USDT本位交易
- 3倍杠杆
- 作为Taker执行

### 3. 现代化技术栈
- **配置管理**: 使用 [Viper](https://github.com/spf13/viper) 支持多种配置源
- **日志系统**: 使用 [Uber Zap](https://github.com/uber-go/zap) 高性能结构化日志
- **日志轮转**: 集成 [Lumberjack](https://github.com/natefinch/lumberjack) 自动日志轮转
- **构建工具**: 完善的 Makefile 支持

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

必填配置:
- `lighter.api_key`: Lighter API密钥
- `lighter.secret_key`: Lighter Secret密钥
- `lighter.private_key`: 十六进制格式的私钥 (80个字符，40字节)

可选配置(有默认值):
- `lighter.base_url`: API地址 (默认: https://api.lighter.xyz)
- `lighter.chain_id`: 链ID (默认: 1)
- `lighter.account_index`: 账户索引 (默认: 1)
- `lighter.api_key_index`: API密钥索引 (默认: 0)
- `trading.usdt_amount`: USDT交易数量 (默认: 1000)
- `trading.leverage`: 杠杆倍数 (默认: 3)
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

### 交易规格
- **交易方式**: USDT本位合约
- **固定金额**: 每次1000 USDT
- **杠杆倍数**: 3倍杠杆
- **订单类型**: 市价单 (作为Taker)

### 市场索引
- BTC: 0
- ETH: 1

### 订单类型
- MarketOrder: 市价单
- ImmediateOrCancel: 立即执行或取消

### 订单方向
- IsAsk = 0: 买入(做多)
- IsAsk = 1: 卖出(做空)

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

## 技术实现

### 核心技术栈
- **Lighter Go SDK**: 官方SDK (`github.com/elliottech/lighter-go`)
- **配置管理**: Viper - 支持YAML文件、环境变量、多种配置源
- **日志系统**: Uber Zap - 高性能结构化日志
- **日志轮转**: Lumberjack - 自动日志文件轮转和压缩
- **构建工具**: 完善的Makefile支持

### 配置系统特性
- **多配置源**: 支持YAML文件、环境变量、默认值
- **优先级**: 环境变量 > 配置文件 > 默认值
- **自动加载**: 自动搜索多个路径下的配置文件
- **类型安全**: 强类型配置结构体
- **验证机制**: 配置项有效性验证

### 日志系统特性
- **结构化日志**: JSON格式便于解析和分析
- **多输出**: 同时输出到控制台和文件
- **日志级别**: debug, info, warn, error
- **自动轮转**: 按大小、时间自动轮转日志文件
- **性能优化**: 零分配的高性能日志记录

### 签名机制
使用Poseidon哈希和Schnorr签名来确保交易安全性。

### 交易流程
1. 加载配置 (支持多种配置源)
2. 初始化日志系统
3. 创建订单请求
4. 生成交易参数
5. 签名交易
6. 记录详细日志
7. 返回交易信息

## 注意事项

1. **安全性**: 请妥善保管你的私钥和API密钥
2. **网络**: 确保网络连接稳定
3. **余额**: 确保账户有足够余额执行交易
4. **测试**: 建议先在测试环境中验证功能

## 依赖项

### 核心依赖
- `github.com/elliottech/lighter-go`: Lighter Go SDK
- `github.com/spf13/viper`: 配置管理库
- `go.uber.org/zap`: 高性能日志库
- `gopkg.in/natefinch/lumberjack.v2`: 日志轮转库

### 间接依赖
- `github.com/elliottech/poseidon_crypto`: Poseidon加密库
- `github.com/ethereum/go-ethereum`: 以太坊相关工具
- `github.com/fsnotify/fsnotify`: 文件系统监控
- `go.uber.org/multierr`: 多错误处理

## 许可证

MIT License