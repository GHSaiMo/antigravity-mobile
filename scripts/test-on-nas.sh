#!/usr/bin/env bash
# 在 Linux 环境（SSH 主机 nas 上的 Docker 容器）中复现 CI 的 Go 校验：
#   go vet ./... && go build ./... && go test -count=1 ./...
#
# 为什么需要：本机是 macOS，GitHub CI 是 ubuntu。路径约定、符号链接、可用工具等平台差异
# 会让“本机全绿、CI 变红”（例如测试写死了 macOS 的 Library/Application Support 路径）。
#
# 用法：
#   scripts/test-on-nas.sh                       # 等价于 CI：vet + build + test ./...
#   scripts/test-on-nas.sh ./internal/cockpit    # 只对指定包跑 test（仍会先跑全量 vet/build）
#   scripts/test-on-nas.sh -run TestFoo ./internal/proxy
#
# 可用环境变量（均有默认值）：
#   NAS_HOST         SSH 主机别名                       默认 nas
#   NAS_WORKDIR      NAS 上的工作目录（代码/工具链/缓存）  默认 /tmp/mgy-ci
#   NAS_GOPROXY      NAS 上可访问的 Go 模块代理           默认 http://127.0.0.1:10001
#   NAS_IMAGE        运行测试的镜像（需带 python 或 sqlite3）默认 muccg/devpi:latest
#   NAS_TOOLCHAIN_IMAGE  解压工具链用的镜像（需 busybox unzip） 默认 alpine:latest
#
# 工作方式：Go 工具链通过 NAS_GOPROXY 以 golang.org/toolchain 模块下载并缓存（版本取自 go.mod），
# 依赖同样走该代理；测试在一次性容器里以普通用户（uid 1001）+ 临时 HOME 运行，用完即删。
# 同步的是本地工作区（含未提交改动），不含 android/ ios/ images/ 等与 Go 无关的目录。
set -euo pipefail

NAS_HOST="${NAS_HOST:-nas}"
NAS_WORKDIR="${NAS_WORKDIR:-/tmp/mgy-ci}"
NAS_GOPROXY="${NAS_GOPROXY:-http://127.0.0.1:10001}"
NAS_IMAGE="${NAS_IMAGE:-muccg/devpi:latest}"
NAS_TOOLCHAIN_IMAGE="${NAS_TOOLCHAIN_IMAGE:-alpine:latest}"

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "${PROJECT_DIR}"

GO_VERSION="$(awk '/^go /{print $2; exit}' go.mod)"
if [ -z "${GO_VERSION}" ]; then
    echo "❌ 无法从 go.mod 读取 Go 版本" >&2
    exit 2
fi
TOOLCHAIN_MOD="v0.0.1-go${GO_VERSION}.linux-amd64"
TOOLCHAIN_DIR="${NAS_WORKDIR}/toolchain/golang.org/toolchain@${TOOLCHAIN_MOD}"

SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "${NAS_HOST}")

if ! "${SSH[@]}" true 2>/dev/null; then
    echo "❌ 无法 SSH 到 ${NAS_HOST}。Linux 验证未执行，不能视为通过。" >&2
    exit 3
fi

echo "▶ [1/4] 准备 NAS 工作目录与 Go ${GO_VERSION} 工具链 (${NAS_HOST}:${NAS_WORKDIR})"
"${SSH[@]}" "mkdir -p '${NAS_WORKDIR}/cache' '${NAS_WORKDIR}/gopath' && chmod a+rwx '${NAS_WORKDIR}' '${NAS_WORKDIR}/cache' '${NAS_WORKDIR}/gopath'"
if ! "${SSH[@]}" "test -x '${TOOLCHAIN_DIR}/bin/go'"; then
    echo "  下载工具链 golang.org/toolchain@${TOOLCHAIN_MOD}（首次请求代理可能先返回 404，会自动重试）"
    "${SSH[@]}" bash -s <<EOF
set -e
cd '${NAS_WORKDIR}'
ok=0
for i in 1 2 3 4 5 6; do
    code=\$(curl -s -m 600 -o go-toolchain.zip -w '%{http_code}' '${NAS_GOPROXY}/golang.org/toolchain/@v/${TOOLCHAIN_MOD}.zip' || true)
    if [ "\$code" = "200" ]; then ok=1; break; fi
    echo "  代理返回 \$code，10 秒后重试 (\$i/6)"; sleep 10
done
[ "\$ok" = "1" ] || { echo "❌ 无法从 ${NAS_GOPROXY} 下载 Go 工具链" >&2; exit 4; }
rm -rf toolchain && mkdir toolchain
docker run --rm -v '${NAS_WORKDIR}:/work' '${NAS_TOOLCHAIN_IMAGE}' unzip -q /work/go-toolchain.zip -d /work/toolchain
rm -f go-toolchain.zip
chmod -R a+rX toolchain
test -x '${TOOLCHAIN_DIR}/bin/go'
EOF
fi

echo "▶ [2/4] 同步本地工作区到 NAS（含未提交改动）"
# 以文件清单打包：已跟踪 + 未忽略的新文件，排除与 Go 无关的大目录和已删除文件。
FILELIST="$(mktemp)"
trap 'rm -f "${FILELIST}"' EXIT
git ls-files -z --cached --others --exclude-standard \
    | tr '\0' '\n' \
    | grep -Ev '^(android|ios|images|logs|bin)/' \
    | while IFS= read -r f; do [ -e "$f" ] && printf '%s\n' "$f"; done > "${FILELIST}"
COPYFILE_DISABLE=1 tar -cf - -T "${FILELIST}" \
    | "${SSH[@]}" "rm -rf '${NAS_WORKDIR}/repo' && mkdir -p '${NAS_WORKDIR}/repo' && tar -xf - -C '${NAS_WORKDIR}/repo' 2>/dev/null; chmod -R a+rwX '${NAS_WORKDIR}/repo'"

echo "▶ [3/4] 在 Linux 容器中执行 vet / build / test"
GO_TEST_ARGS=("$@")
if [ "${#GO_TEST_ARGS[@]}" -eq 0 ]; then
    GO_TEST_ARGS=("./...")
fi
# 将参数安全地传入远端 shell
QUOTED_ARGS="$(printf '%q ' "${GO_TEST_ARGS[@]}")"

set +e
"${SSH[@]}" "docker run --rm --network host -u 1001:1001 \
  -e HOME=/home/runner -e CI=true \
  -e GOROOT='/work/toolchain/golang.org/toolchain@${TOOLCHAIN_MOD}' \
  -e GOTOOLCHAIN=local -e GOPROXY='${NAS_GOPROXY}' -e GOSUMDB=off -e GOFLAGS=-mod=mod -e CGO_ENABLED=0 \
  -e GOCACHE=/work/cache -e GOPATH=/work/gopath \
  --tmpfs /home/runner:rw,exec,uid=1001,gid=1001,mode=0755 \
  -v '${NAS_WORKDIR}:/work' -w /work/repo \
  --entrypoint sh '${NAS_IMAGE}' -c 'export PATH=\$GOROOT/bin:\$PATH; \
    go version && echo --- go vet && go vet ./... && echo --- go build && go build ./... \
    && echo --- go test && go test -count=1 ${QUOTED_ARGS//\'/\'\\\'\'}'"
RC=$?
set -e

echo "▶ [4/4] 结果"
if [ "${RC}" -eq 0 ]; then
    echo "✅ Linux 验证通过（对齐 CI：vet + build + test）"
else
    echo "❌ Linux 验证失败（退出码 ${RC}）" >&2
fi
exit "${RC}"
