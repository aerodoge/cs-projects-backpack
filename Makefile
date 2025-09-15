.PHONY: help build run test clean daemon restart status logs fmt lint deps dev-deps ci

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
BINARY_NAME=lighter-trader
BUILD_DIR=build
PID_FILE=$(BINARY_NAME).pid
LOG_FILE=logs/app.log

# 默认目标
help:
	@echo "🚀 Lighter Exchange Trading Bot - Makefile 命令"
	@echo ""
	@echo "📦 构建和运行:"
	@echo "  deps      - 整理依赖"
	@echo "  build     - 编译程序"
	@echo "  run       - 前台运行程序"
	@echo "  daemon    - 后台运行程序"
	@echo "  stop      - 停止后台程序"
	@echo "  restart   - 重启程序"
	@echo "  status    - 查看程序状态"
	@echo "  logs      - 查看实时日志"
	@echo ""
	@echo "🧹 维护:"
	@echo "  clean     - 清理编译文件"
	@echo "  test      - 运行测试"
	@echo "  fmt       - 格式化代码"
	@echo "  lint      - 代码检查"
	@echo ""
	@echo "🔧 开发:"
	@echo "  dev-deps  - 安装开发依赖"
	@echo "  ci        - 完整CI检查"
	@echo ""

# 依赖管理
deps:
	@echo "整理依赖..."
	go mod tidy
	@echo "✅ 依赖整理完成"

# 编译
build: deps
	@echo "编译程序..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p logs
	go build -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) cmd/main.go
	@echo "✅ 编译完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 前台运行
run: build
	@echo "启动程序（前台）..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# 后台运行
daemon: build
	@echo "启动程序（后台）..."
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "程序已在运行，PID: $$PID"; \
			exit 1; \
		else \
			rm -f $(PID_FILE); \
		fi \
	fi
	@nohup ./$(BUILD_DIR)/$(BINARY_NAME) >> $(LOG_FILE) 2>&1 & echo $$! > $(PID_FILE)
	@sleep 1
	@if kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "✅ 程序已后台启动，PID: $$(cat $(PID_FILE))"; \
		echo "日志: $(LOG_FILE)"; \
	else \
		echo "❌ 程序启动失败"; \
		rm -f $(PID_FILE); \
		exit 1; \
	fi

# 停止程序
stop:
	@if [ ! -f $(PID_FILE) ]; then \
		echo "程序未在运行"; \
	else \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "停止程序，PID: $$PID"; \
			kill $$PID; \
			sleep 2; \
			if kill -0 $$PID 2>/dev/null; then \
				echo "强制停止程序"; \
				kill -9 $$PID; \
			fi; \
			rm -f $(PID_FILE); \
			echo "✅ 程序已停止"; \
		else \
			echo "程序未在运行"; \
			rm -f $(PID_FILE); \
		fi \
	fi

# 查看状态
status:
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ 程序正在运行，PID: $$PID"; \
			ps -o pid,ppid,pcpu,pmem,etime,cmd -p $$PID; \
		else \
			echo "❌ 程序未在运行（PID文件存在但进程已死）"; \
			rm -f $(PID_FILE); \
		fi \
	else \
		echo "❌ 程序未在运行"; \
	fi

# 查看日志
logs:
	@if [ -f $(LOG_FILE) ]; then \
		echo "实时日志 (Ctrl+C 退出):"; \
		tail -f $(LOG_FILE); \
	else \
		echo "日志文件不存在: $(LOG_FILE)"; \
	fi

# 重启
restart: stop
	@sleep 2
	@$(MAKE) daemon

# 测试
test:
	go test -v ./...

# 清理
clean:
	@echo "清理编译文件..."
	rm -rf $(BUILD_DIR)/*
	rm -f $(PID_FILE)
	@echo "✅ 清理完成"

# 代码格式化
fmt:
	@echo "格式化代码..."
	go fmt ./...
	@command -v goimports >/dev/null 2>&1 && goimports -w . || echo "goimports 未安装，跳过导入整理"
	@echo "✅ 代码格式化完成"

# 代码检查
lint:
	@echo "运行代码检查..."
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 || { echo "请先安装 golangci-lint"; exit 1; }
	golangci-lint run
	@echo "✅ 代码检查完成"

# 安装开发依赖
dev-deps:
	@echo "安装开发依赖..."
	@command -v golangci-lint >/dev/null 2>&1 || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.54.2
	@command -v goimports >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	@echo "✅ 开发依赖安装完成"

# 运行完整的 CI 检查
ci: fmt lint test build
	@echo "✅ 完整 CI 检查完成"