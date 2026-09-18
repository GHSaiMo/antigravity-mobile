.PHONY: build run test clean tmux-start tmux-stop pair list clear clear-all install-local

# Build the unified single binary with embedded web assets
build:
	@mkdir -p bin
	go build -ldflags="-s -w -X 'main.Version=1.0.0'" -o bin/mgy ./cmd/gateway
	@ln -sf mgy bin/gateway
	@codesign -s - -f bin/mgy 2>/dev/null || true
	@echo "Build complete: bin/mgy (symlinked as bin/gateway)"

# Install mgy to ~/.local/bin/mgy for quick local testing
install-local: build
	@mkdir -p $(HOME)/.local/bin
	@cp bin/mgy $(HOME)/.local/bin/mgy
	@codesign -s - -f $(HOME)/.local/bin/mgy 2>/dev/null || true
	@echo "Installed mgy to $(HOME)/.local/bin/mgy"

# ==============================================================================
# 本地前台运行网关 (make run)
#
# 启动项参数配置说明 (可在执行 make run 时通过环境变量或参数传入覆盖):
#
# 1. 命令行参数 (CLI Flags):
#    -port <端口号>        : 网关服务 HTTP/WebSocket 监听端口，默认 58900 (支持环境变量: MULTIGRAVITY_PORT / GATEWAY_PORT)
#    -host <主机/IP>       : 监听地址，默认 "" 自动双栈监听本机所有 IPv4 与 IPv6 接口；
#                            若设为 127.0.0.1 则仅限本机访问 (支持环境变量: MULTIGRAVITY_HOST / GATEWAY_HOST)
#    -qr=<true|false>     : 启动时是否在控制台默认打印一次客户端配对二维码，默认 true (可通过 -qr=false 关闭)
#    -poll <秒数>          : 探测本地 Antigravity 实例与健康检查的轮询间隔秒数，默认 5 秒
#    -ddns <域名/IP>       : 公网 DDNS 域名或固定 IPv6 地址，用于生成扫码配对 URI 及外部直连 (支持环境变量: DDNS_HOST)
#    -ssl                 : 是否在配对链接与服务中启用 HTTPS/SSL 协议，默认 false
#                            开启需配合 -tls-cert 与 -tls-key 使用 (支持环境变量: GATEWAY_SSL=1)
#    -tls-cert <文件路径>  : HTTPS TLS 证书文件路径 (.cer/.crt/.pem，支持环境变量: TLS_CERT_FILE)
#    -tls-key <文件路径>   : HTTPS TLS 私钥文件路径 (.key，支持环境变量: TLS_KEY_FILE)
#
# 2. 常用环境变量 (推荐在 ~/.multigravity/.env 或项目根目录 .env 中按需配置):
#    BARK_URL             : iOS Bark 实时推送链接（填入后自动开启任务完成与审批推送通知）
#    FRP_ENABLE           : 是否启用 FRP 内网穿透云中继通道 (1 开启, 0 关闭)
#    FRP_SERVER_ADDR      : FRP 远程服务器 IP 或公网域名
#    FRP_SERVER_PORT      : FRP 远程服务器通信端口 (默认 7000)
#    FRP_TOKEN            : FRP 鉴权密钥 Token
#    FRP_REMOTE_PORT      : FRP 映射的远程公网访问端口 (默认 58900)
#    ADMIN_TOKEN          : 管理员特权密钥 (外网访问或开启 FRP 时用于鉴权与管理设备)
#    AUTH_STORE_PATH      : 设备凭据持久化存储路径 (默认 ~/.multigravity/auth_store.json)
#    INCLUDE_PUBLIC_IPV6  : 手机蜂窝网络下是否使用公网 IPv6 地址直连配对 (1 开启, 0 关闭)
#
# 常见运行方式示例:
#    make run                           # 默认启动，监听 58900 端口，并默认打印一次配对二维码
#    make run PORT=58901                # 自定义监听端口为 58901
#    make run ARGS="-qr=false"          # 启动网关但不打印配对二维码
#    make run ARGS="-host 127.0.0.1"    # 仅限本机 loopback 访问
# ==============================================================================
PORT ?= 58900
ARGS ?=

run: build
	./bin/mgy -port $(PORT) $(ARGS)

# Run all unit and integration tests
test:
	go test -v ./cmd/... ./internal/...

# Clean artifacts
clean:
	rm -rf bin dist build logs/*.log

# Launch gateway in a detached tmux session with log tee
tmux-start: build
	@mkdir -p logs
	@./scripts/tmux-start.sh

# Stop the tmux session
tmux-stop:
	@./scripts/tmux-stop.sh

# Display a fresh pairing QR code and URI in the terminal
pair: build
	@./bin/mgy pair

# List all paired mobile devices (works online & offline)
list: build
	@./bin/mgy list

# Clear paired devices (supports: make clear all, make clear-all, make clear)
clear-all: build
	@./bin/mgy clear all

clear: build
	@./bin/mgy clear $(if $(DEVICE),$(DEVICE),$(filter-out clear,$(MAKECMDGOALS)))

# Prevent make error when user executes `make clear all`
ifeq (clear,$(firstword $(MAKECMDGOALS)))
  all:
	@:
endif

