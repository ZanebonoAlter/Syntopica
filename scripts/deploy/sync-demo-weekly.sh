#!/usr/bin/env bash
# 每周演示环境 seed 定时同步 wrapper（由用户级 sync-demo-seed.timer 触发，也可手动跑）。
#
# 链路：阶段[防呆](负载/磁盘阈值) → 阶段[导出](dump-sanitizer 脱敏导出)
#   → 阶段[断言](三断言 fail-closed) → 阶段[归档](轮换保留最近 N 份)
#   → 阶段[部署](deploy-remote.sh --demo：构建→推送→拉起) → 阶段[health](复检)。
# 任一环节非零退出即终止后续环节，不向演示机推送任何文件（无半程推送）；
# 每阶段一行状态日志（阶段[x] 前缀），journal 友好：journalctl --user -u sync-demo-seed。
#
# 安全断言四条目（前三条来自 demo/seed/README.md Security review；第四条为
# 2026-09-24 泄露事故后新增）：
#   1. 不含 INSERT INTO ai_call_logs      （请求/响应快照属敏感数据）
#   2. 不含 INSERT INTO schema_migrations （会压制新库迁移）
#   3. ai_providers 各行 api_key 为空     （凭据不落 demo）
#   4. RSSHub 改写源 host 无残留          （改写未生效时自托管地址会直进公开 demo）
# 断言不过宁可不同步——演示数据旧一周，也不推带 api_key/私有地址的快照。
#
# RSSHub 改写规则（RSSHUB_REWRITE，格式 源host=目标host）：环境变量优先，
# 其次读 demo/seed/.rsshub-rewrite（gitignore，含私有基础设施信息不入库）。
# 两者皆无时 fail-closed 中止导出（除非显式 ALLOW_NO_RSSHUB_REWRITE=1）。
#
# 可用环境变量：
#   LOAD_THRESHOLD   防呆负载阈值（默认 4，Pi 4 核满载线；临时调低可验证跳过路径）
#   DISK_MIN_GB      demo/seed 所在盘最小余量 GB（默认 5）
#   ARCHIVE_KEEP     归档保留份数（默认 2）
#   SEED_FILE        断言/归档对象（默认 <repo>/demo/seed/seed.sql；测试注入用）
#   DEMO_HEALTH_URL  部署后健康复检地址（默认 http://10.11.12.59:5080/health）
#   SYNC_SKIP_EXPORT 测试钩子：置 1 跳过导出阶段（毒样本断言演练用，生产勿用）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SEED_FILE="${SEED_FILE:-$REPO_ROOT/demo/seed/seed.sql}"
SEED_DIR="$(dirname "$SEED_FILE")"
DEMO_HEALTH_URL="${DEMO_HEALTH_URL:-http://10.11.12.59:5080/health}"
LOAD_THRESHOLD="${LOAD_THRESHOLD:-4}"
DISK_MIN_GB="${DISK_MIN_GB:-5}"
ARCHIVE_KEEP="${ARCHIVE_KEEP:-2}"
RSSHUB_REWRITE_FILE="$SEED_DIR/.rsshub-rewrite"

# systemd 用户单元的 PATH 不含 go 安装位，补上（已有 go 则幂等无操作）
command -v go >/dev/null 2>&1 || export PATH="$PATH:/usr/local/go/bin"

log() { printf '[sync-demo-seed] %s\n' "$*"; }
ok()  { log "✓ $*"; }
die() { log "✗ $*"; exit 1; }

# ── RSSHub 改写规则加载：环境变量 > 配置文件 > fail-closed 中止 ──
load_rsshub_rewrite() {
  if [ -n "${RSSHUB_REWRITE:-}" ]; then
    log "RSSHub 改写规则：来自环境变量（${RSSHUB_REWRITE%%=*} → ${RSSHUB_REWRITE##*=}）"
    return 0
  fi
  if [ -s "$RSSHUB_REWRITE_FILE" ]; then
    export RSSHUB_REWRITE="$(head -1 "$RSSHUB_REWRITE_FILE" | tr -d '[:space:]')"
    log "RSSHub 改写规则：来自 $RSSHUB_REWRITE_FILE"
    return 0
  fi
  if [ "${ALLOW_NO_RSSHUB_REWRITE:-0}" = "1" ]; then
    log "⚠ 未配置 RSSHub 改写规则，ALLOW_NO_RSSHUB_REWRITE=1 放行（请确认真库无自托管源）"
    return 0
  fi
  return 1
}

# ── 阶段[防呆]：负载/磁盘双阈值，超限跳过（退出 0 不算失败，演示环境保持现版本，下周期自愈）
precheck() {
  local load1 disk_gb
  load1="$(cut -d' ' -f1 /proc/loadavg)"
  if awk -v l="$load1" -v t="$LOAD_THRESHOLD" 'BEGIN { exit !(l > t) }'; then
    log "SKIP 阶段[防呆]: load ${load1} > 阈值 ${LOAD_THRESHOLD}，本机忙，本次跳过（下周期自愈）"
    exit 0
  fi
  disk_gb="$(df -BG --output=avail "$SEED_DIR" | tail -1 | tr -dc '0-9')"
  if [ "$disk_gb" -lt "$DISK_MIN_GB" ]; then
    log "SKIP 阶段[防呆]: 磁盘余量 ${disk_gb}GB < ${DISK_MIN_GB}GB，本次跳过（下周期自愈）"
    exit 0
  fi
  ok "阶段[防呆] 通过（load=${load1}≤${LOAD_THRESHOLD}，disk=${disk_gb}GB≥${DISK_MIN_GB}GB）"
}

# ── 阶段[断言]：三断言逐项检查，任一命中打印命中项并返回非零（fail-closed）
# 用法: assert_seed_safe <seed.sql 路径>
assert_seed_safe() {
  local f="$1" hit
  [ -s "$f" ] || { log "✗ 断言对象不存在或为空: $f"; return 1; }

  # 断言 1/2：敏感表整表不得出现（INSERT 头固定在行首，行级 grep 即可命中）
  hit="$(grep -n -m1 'INSERT INTO ai_call_logs' "$f" || true)"
  if [ -n "$hit" ]; then
    log "✗ 断言失败 [ai_call_logs]: seed 含 ai_call_logs 导出（请求/响应快照）"
    log "  命中: ${hit:0:160}"
    return 1
  fi
  hit="$(grep -n -m1 'INSERT INTO schema_migrations' "$f" || true)"
  if [ -n "$hit" ]; then
    log "✗ 断言失败 [schema_migrations]: seed 含 schema_migrations 导出（会压制新库迁移）"
    log "  命中: ${hit:0:160}"
    return 1
  fi

  # 断言 4：改写源 host 不得残留（feeds.url/icon 漏改时自托管地址直进公开 demo）
  if [ -n "${RSSHUB_REWRITE:-}" ]; then
    local src_host="${RSSHUB_REWRITE%%=*}"
    hit="$(grep -n -m1 -F "$src_host" "$f" || true)"
    if [ -n "$hit" ]; then
      log "✗ 断言失败 [rsshub-host]: seed 残留改写源 host（RSSHub 改写未生效？）"
      log "  命中: ${hit:0:160}"
      return 1
    fi
    rsshub_ok=1
  else
    rsshub_ok=0
  fi

  # 断言 3：ai_providers 行内 api_key 列值检查。
  # 不能全文件裸 grep 'api_key'——articles 正文含 [redacted-token] 字样会误伤；
  # 用引号感知状态机只解析 INSERT INTO ai_providers 语句块：从列名表定位 api_key
  # 列序，逐元组取值，非 NULL/'' 即命中；语句未终结（文件截断）同样判失败。
  if ! awk '
    function fail(msg) {
      printf "✗ 断言失败 [api_key]: %s\n", msg > "/dev/stderr"
      bad = 1; exit 4
    }
    function check_tuple(t,   fields, fn, i, c, out, val, n) {
      if (apikey_idx < 1) return
      fn = 0; out = ""; inq = 0; n = length(t)
      for (i = 1; i <= n; i++) {
        c = substr(t, i, 1)
        if (c == q) {
          if (inq) {
            if (i < n && substr(t, i + 1, 1) == q) { out = out q q; i++; continue }
            inq = 0
          } else { inq = 1 }
          out = out c; continue
        }
        if (!inq && c == ",") { fields[fn++] = out; out = ""; continue }
        out = out c
      }
      fields[fn] = out
      if (fn < apikey_idx - 1) fail("ai_providers 元组列数不足（解析异常）: " substr(t, 1, 60))
      val = fields[apikey_idx - 1]
      gsub(/^[ \t\n]+|[ \t\n]+$/, "", val)
      if (val != "NULL" && val != "" && val != (q q))
        fail("ai_providers 行携带非空 api_key: " substr(val, 1, 60))
    }
    function do_scan(s,   p, c, n) {
      n = length(s)
      for (p = 1; p <= n; p++) {
        if (mode == 0) return
        c = substr(s, p, 1)
        if (inq) {
          tuplebuf = tuplebuf c
          if (c == q) {
            if (p < n && substr(s, p + 1, 1) == q) { p++; tuplebuf = tuplebuf q; continue }
            inq = 0
          }
          continue
        }
        if (c == q) { inq = 1; tuplebuf = tuplebuf c; continue }
        if (c == "(") { depth++; if (depth == 1) tuplebuf = ""; else tuplebuf = tuplebuf c; continue }
        if (c == ")") {
          depth--
          if (depth == 0) check_tuple(tuplebuf)
          else tuplebuf = tuplebuf c
          continue
        }
        if (c == ";" && depth == 0) { mode = 0; return }
        tuplebuf = tuplebuf c
      }
    }
    BEGIN { q = sprintf("%c", 39); mode = 0; inq = 0; depth = 0; tuplebuf = ""; bad = 0 }
    {
      if (mode == 0) {
        if ($0 ~ /^INSERT INTO ai_providers \(/) {
          hdr = $0
          sub(/^INSERT INTO ai_providers \(/, "", hdr)
          sub(/\).*/, "", hdr)
          apikey_idx = -1
          ncols = split(hdr, cn, ",")
          for (i = 1; i <= ncols; i++) {
            gsub(/^[ \t]+|[ \t]+$/, "", cn[i])
            if (cn[i] == "api_key") apikey_idx = i
          }
          if (apikey_idx == -1)
            printf "⚠ ai_providers 列名表无 api_key 列（schema 变更？），跳过列值检查\n" > "/dev/stderr"
          mode = 1; inq = 0; depth = 0; tuplebuf = ""
          do_scan(substr($0, index($0, "VALUES") + 6))
        }
        next
      }
      do_scan($0)
    }
    END {
      if (bad) exit 4
      if (mode == 1) {
        printf "✗ 断言失败 [api_key]: ai_providers 语句未终结（文件截断？）\n" > "/dev/stderr"
        exit 5
      }
    }
  ' "$f"; then
    log "✗ 断言失败 [api_key]（命中详情见上）"
    return 1
  fi

  if [ "$rsshub_ok" = "1" ]; then
    ok "断言 4/4 通过（无 ai_call_logs、无 schema_migrations、api_key 全空、改写源 host 无残留）"
  else
    ok "断言 3/3 通过（无 ai_call_logs、无 schema_migrations、ai_providers.api_key 全空；未配改写规则，跳过 host 检查）"
  fi
}

# ── 阶段[归档]：轮换删除最旧归档（只留最近 ARCHIVE_KEEP 份；名字含 YYYYMMDD-HHMM，字典序=时间序）
rotate_archives() {
  local list old
  list="$(ls -1 "$SEED_DIR"/seed-*.sql 2>/dev/null | sort || true)"
  [ -n "$list" ] || return 0
  local count
  count="$(printf '%s\n' "$list" | wc -l)"
  [ "$count" -gt "$ARCHIVE_KEEP" ] || return 0
  printf '%s\n' "$list" | head -n -"$ARCHIVE_KEEP" | while IFS= read -r old; do
    log "  轮换删除最旧归档: $(basename "$old")"
    rm -f "$old"
  done
}

archive_seed() {
  local dest
  dest="${SEED_DIR}/seed-$(date +%Y%m%d-%H%M).sql"
  cp "$SEED_FILE" "$dest"
  log "  归档: $(basename "$dest")（$(( $(stat -c%s "$dest") / 1024 / 1024 ))MB）"
  rotate_archives
}

main() {
  log "=== 演示环境 seed 每周同步开始 ==="

  # ① 防呆（超限 SKIP 并以 0 退出——不算失败）
  precheck

  # ② 导出
  if [ "${SYNC_SKIP_EXPORT:-0}" = "1" ]; then
    log "SKIP 阶段[导出]: SYNC_SKIP_EXPORT=1（测试钩子，直接断言现有 seed）"
  else
    load_rsshub_rewrite \
      || die "阶段[导出] 前置检查失败：未配置 RSSHub 改写规则。创建 $RSSHUB_REWRITE_FILE（内容：源host=目标host）或设 RSSHUB_REWRITE 环境变量；确认真库无自托管源可临时 ALLOW_NO_RSSHUB_REWRITE=1"
    log "阶段[导出] 开始：dump-sanitizer 从真库脱敏导出"
    (cd "$REPO_ROOT/backend-go" && SEED_OUT="$SEED_FILE" RSSHUB_REWRITE="${RSSHUB_REWRITE:-}" go run ./cmd/dump-sanitizer) \
      || die "阶段[导出] 失败：真库不可达或导出出错，演示环境保持原样"
    ok "阶段[导出] 完成（$(( $(stat -c%s "$SEED_FILE") / 1024 / 1024 ))MB）"
  fi

  # ③ 断言（失败不归档不部署——毒样本不占回滚位、不出本机）
  log "阶段[断言] 开始：ai_call_logs / schema_migrations / api_key 三断言"
  assert_seed_safe "$SEED_FILE" \
    || die "阶段[断言] 未通过：中止同步，未归档未推送任何文件"
  ok "阶段[断言] 完成：seed 安全，放行"

  # ④ 归档
  log "阶段[归档] 开始：复制归档并轮换保留最近 ${ARCHIVE_KEEP} 份"
  archive_seed || die "阶段[归档] 失败"
  ok "阶段[归档] 完成"

  # ⑤ 部署（deploy-remote.sh 内含远端拉起与健康轮询，失败非零）
  # ⚠ deploy-remote.sh 的 rsync 用相对路径 ./，必须从仓库根目录调用——
  #   systemd 默认 CWD=$HOME，曾因此把整个 HOME 镜像推到演示机（2026-09-24 事故），
  #   故这里显式 cd 到 $REPO_ROOT 再调（unit 里的 WorkingDirectory 是第二道保险）
  log "阶段[部署] 开始：deploy-remote.sh --demo（本机构建镜像→save/load 推送→远端拉起）"
  (cd "$REPO_ROOT" && bash "$REPO_ROOT/scripts/deploy/deploy-remote.sh" --demo) \
    || die "阶段[部署] 失败：演示机保留上一个可用版本（旧 tag 未删，可用 DEMO_TAG 回滚）"
  ok "阶段[部署] 完成"

  # ⑥ 健康复检（deploy-remote 已轮询过一次，这里独立复确认并留阶段日志）
  log "阶段[health] 开始：健康复检 ${DEMO_HEALTH_URL}"
  curl -fsS --noproxy '*' --max-time 15 "$DEMO_HEALTH_URL" >/dev/null \
    || die "阶段[health] 失败：演示机健康检查未过"
  ok "阶段[health] 完成：演示环境已是最新 seed"

  log "=== 演示环境 seed 每周同步全链路完成 ==="
}

# 可 source 单独调用函数（fixture 断言演练 / 归档轮换演练），直接执行才跑主链路
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
