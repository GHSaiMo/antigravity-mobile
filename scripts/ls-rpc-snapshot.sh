#!/usr/bin/env bash
# language_server RPC 清单快照与升级 diff。
#
# 背景：网关对 /exa.language_server_pb.* 整段透传，整个项目建立在逆向之上；
# Antigravity 升级后 language_server 的 RPC 可能新增、改名或消失。本脚本从二进制里
# 抽取 LanguageServerService 方法名，与仓库内基线比对，第一时间发现破坏性变更与新能力。
#
# 用法：
#   scripts/ls-rpc-snapshot.sh              # 等同 --diff：与基线比对，列出新增/消失，并标注项目已使用的
#   scripts/ls-rpc-snapshot.sh --update     # 用当前二进制重写基线（升级核对完后执行）
#   scripts/ls-rpc-snapshot.sh --print      # 仅打印当前二进制的方法清单
#   scripts/ls-rpc-snapshot.sh --diff /path/to/language_server
#
# 二进制路径：第二个参数 > $LS_BIN > macOS 默认路径 > Linux 常见路径。
# 退出码：0 无"项目已使用的方法消失"；1 有已使用的方法消失（需要处理）；3 找不到二进制。
#
# 注意：方法名取自 connect-go 的 LanguageServerServiceHandler.<Method> 字符串。
# 同一基线还被 internal/proxy/ls_rpc_contract_test.go 用来离线校验代码里引用的 RPC 名。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$ROOT/internal/proxy/testdata/ls_rpc_methods.txt"
MODE="${1:---diff}"
BIN="${2:-${LS_BIN:-}}"

if [[ -z "$BIN" ]]; then
  for c in \
    "/Applications/Antigravity.app/Contents/Resources/bin/language_server" \
    "$HOME/.gemini/antigravity/bin/language_server" \
    "/usr/share/antigravity/resources/bin/language_server" \
    "/opt/Antigravity/resources/bin/language_server"; do
    [[ -x "$c" ]] && BIN="$c" && break
  done
fi
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
  echo "找不到 language_server 二进制，请通过第二个参数或 LS_BIN 指定" >&2
  exit 3
fi

extract() {
  # connect-go 生成的 Handler 方法名是独立字符串，比 "服务/方法" 路径形式更干净（无粘连噪声）
  grep -aoE 'LanguageServerServiceHandler\.[A-Za-z]+' "$BIN" \
    | sed 's#.*\.##' | grep -vx 'func' | LC_ALL=C sort -u
}

# 项目源码中引用到的方法名（网关 Go、Android、iOS、Web）
used() {
  (cd "$ROOT" && grep -rhoE 'LanguageServerService/[A-Za-z]+|rpc\("[A-Za-z]+"' \
      --include='*.go' --include='*.kt' --include='*.swift' --include='*.js' \
      --exclude='*_test.go' --exclude-dir=build --exclude-dir=node_modules \
      internal cmd android/app/src ios web 2>/dev/null) \
    | sed -E 's#^LanguageServerService/##; s#^rpc\("##; s#"$##' | LC_ALL=C sort -u
}

# 不要执行 language_server 取版本（它不是普通 CLI，可能直接起服务挂住）
version_line() {
  local v=""
  local plist="${BIN%/Contents/Resources/bin/language_server}/Contents/Info.plist"
  if [[ -f "$plist" ]] && command -v /usr/libexec/PlistBuddy >/dev/null 2>&1; then
    v="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist" 2>/dev/null || true)"
  fi
  [[ -z "$v" ]] && v="$(ps -eo args= | grep -m1 -oE 'override_ide_version [0-9.]+' | awk '{print $2}' || true)"
  echo "${v:-unknown}"
}

case "$MODE" in
  --print) extract ;;
  --update)
    cur="$(extract)"
    { echo "# language_server RPC baseline"
      echo "# generated: $(date +%F) ide_version: $(version_line) count: $(echo "$cur" | wc -l | tr -d ' ')"
      echo "$cur"; } > "$BASELINE"
    echo "已更新基线：$BASELINE（$(echo "$cur" | wc -l | tr -d ' ') 个方法）"
    ;;
  --diff)
    [[ -f "$BASELINE" ]] || { echo "基线不存在，先执行 --update" >&2; exit 3; }
    tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
    grep -v '^#' "$BASELINE" | LC_ALL=C sort -u > "$tmp/old"
    extract > "$tmp/new"; used > "$tmp/used"
    LC_ALL=C comm -13 "$tmp/old" "$tmp/new" > "$tmp/added"
    LC_ALL=C comm -23 "$tmp/old" "$tmp/new" > "$tmp/removed"
    echo "基线：$(sed -n 2p "$BASELINE")"
    echo "当前：$(date +%F) ide_version: $(version_line) count: $(wc -l < "$tmp/new" | tr -d ' ')"
    echo; echo "== 新增 ($(wc -l < "$tmp/added" | tr -d ' ')) =="; cat "$tmp/added"
    echo; echo "== 消失 ($(wc -l < "$tmp/removed" | tr -d ' ')) =="
    broken=0
    while read -r m; do
      [[ -z "$m" ]] && continue
      if grep -qx "$m" "$tmp/used"; then echo "$m   <-- 项目正在使用！"; broken=1; else echo "$m"; fi
    done < "$tmp/removed"
    echo
    # 项目使用但当前二进制里找不到（已知的非 LanguageServerService 名字见 KNOWN_NON_LS）
    KNOWN_NON_LS=" FindFiles GetFileDetails IM SetCascadeTrajectoryMetadata "
    while read -r u; do
      [[ "$KNOWN_NON_LS" == *" $u "* ]] && continue
      grep -qx "$u" "$tmp/new" || { echo "!! 已使用的 $u 不在当前二进制中"; broken=1; }
    done < "$tmp/used"
    [[ $broken -eq 0 ]] && echo "✅ 项目使用的 RPC 均仍存在" || { echo "❌ 存在已使用 RPC 消失，请排查"; exit 1; }
    ;;
  *) echo "未知参数: $MODE" >&2; exit 2 ;;
esac
