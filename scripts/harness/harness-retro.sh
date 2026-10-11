#!/usr/bin/env bash
# harness-retro.sh — harness 事实账本的失败聚类复盘报告（change: harness-retro-loop）
#
# 用法：bash scripts/harness/harness-retro.sh [选项]
#   --db PATH              目标账本（缺省 .pi/harness/events.db）
#   --days N               统计窗口天数（缺省 7）
#   --change NAME          只统计 change 列归属该值的事件（缺省全局）
#   --lifecycle NAME       单 change 生命周期体检卡（六段：流程规范/UI 工作流/成本卡/健康度/人类反馈/
#                          review 覆盖 + proposal 头注释声明元数据；不受 --days 窗口限制，TTL 30 天为硬上限；
#                          add-change-lifecycle-retro + add-review-trace-gate）
#   --warn-threshold N     软提醒失效阈值（缺省 5）
#   --json                 输出机器可读 JSON（与文本同结构）
#   --save-baseline [PATH] 落基线快照（缺省 .pi/harness/retro-baseline.json）
#   --baseline [PATH]      与基线比对并输出变化量（同上缺省路径）
#   --help                 打印用法
#
# 只读消费：sqlite3 -readonly 打开 + PRAGMA application_id 校验（魔数 0x53594E54），
# 不建表、不写事件、不触碰注入通道配置（报告只以命令输出形态存在）。
# 六段故障视角 + ⑦效能看板（harness-effectiveness-metrics）：① 门禁失败聚类 ② 回归翻转 ③ harness 自身故障 ④ 软提醒失效 ⑤ 重复失败热点 ⑥ 注入面健康 ⑦ 效能看板（A 插件 ROI / B 效率基线（session.rollup 驱动）/ C 返工信号；分布为 nearest-rank 分位，禁止综合总分）
# ⑦ 另含 D 测试欠账巡检子组（test-debt-patrol）：patrol.check 流水频次 + test_debt 台账 open/fixed/waived 趋势；
#   窗口内无 patrol.check 或库内无 test_debt 表（老库/未初始化）时降级为「无巡检数据」，不影响退出码。
#
# 分母口径（与 harness-fact-log 的 gate.check 采样记账协议一致）：
#   ok=false 计 1；ok=true 无采样标记（会话首成功/转绿锚点）计 1；
#   ok=true 且 sampled=true 按 payload.n 计（缺 n 回退缺省 N=5；n 非法保守计 1 并告警）。
#
# 退出码：0 报告成功（含窗口内无数据、发现大量失败）；1 参数或环境错误（库缺失/非本应用库/损坏）。
# 依赖：bash、sqlite3（≥3.33，含 json1 与窗口函数）、python3（渲染与基线读写）、awk 不需要。
# 契约锚：openspec/changes/harness-retro-loop/specs/harness-retro-loop/spec.md
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

SCRIPT_VERSION="1"
APP_ID_DEC=1398361684 # 0x53594E54 = 'S''Y''N''T'
DEFAULT_SAMPLE_N=5    # 与 .pi/extensions/lib/gate-sample.ts 的 GATE_OK_SAMPLE_EVERY 一致
TTL_DAYS_MIN=30       # 被消费 kind 的最短保留期（session.start 为 90 天）
HOTSPOT_DEFAULT_THR=5

# 情报型 reasonCode 白名单（④ 段分类，split-retro-warn-classes）：归属/状态告知类 warn——
# 告知"这个红不全是你弄的"等客观状态（如跨 change 并发撞车、外部破坏），不期待 agent 的
# 服从动作，故不计入软提醒失效判定，只单列计数供并发频率/成本趋势观察。白名单与
# harness-facts skill 的 reasonCode 词汇表同步演进；白名单之外的 reasonCode 一律缺省按
# 纠正型处理（最强监督口径：新情报型词汇须显式登记入此白名单，防静默漏进失效名单；
# 发现误归类时修白名单，不放宽缺省）。
# attribution-degraded（add-foreign-gate-steer-attribution）：归因信号缺席/判定异常而保守
# 回退本会话失败的告知性记账——考古"为何判 mine"用，不构成催修指令。
INFORMATIVE_REASONS="concurrent-mixed concurrent-dirty-tree foreign-breakage attribution-degraded"
# SQL IN 字面量（仅含 [a-z0-9-]，无需引号转义）：'concurrent-mixed','…','…'
INFORMATIVE_SQL_LIST="$(printf "'%s'," $INFORMATIVE_REASONS | sed 's/,$//')"

DB=".pi/harness/events.db"
DAYS=7
CHANGE=""
LIFECYCLE=""
WARN_THRESHOLD=$HOTSPOT_DEFAULT_THR
JSON_MODE=0
SAVE_BASELINE=""
BASELINE=""
BASELINE_DEFAULT=".pi/harness/retro-baseline.json"

usage() {
	cat <<'USAGE'
harness-retro — harness 事实账本的失败聚类复盘报告（只读）

用法: bash scripts/harness/harness-retro.sh [选项]

  --db PATH               目标账本（缺省 .pi/harness/events.db）
  --days N                统计窗口天数（缺省 7；超过 30 天会提示 TTL 清扫风险）
  --change NAME           只统计 change 列归属该值的事件（缺省全局）
  --lifecycle NAME        单 change 生命周期体检卡（六段 + proposal 声明元数据；
                          与 --change / 基线选项互斥，不受 --days 窗口限制）
  --warn-threshold N      纠正型软提醒失效阈值（缺省 5；纠正型 warn 计数 ≥ 阈值即列出；
                          仅作用于纠正型失效判定，情报型 warn 单列计数不受影响）
  --json                  输出机器可读 JSON
  --save-baseline [PATH]  落基线快照（缺省 .pi/harness/retro-baseline.json）
  --baseline [PATH]       与基线比对并输出变化量（同上缺省路径）
  --help                  打印本用法

退出码: 0 报告成功（含窗口内无数据）；1 参数或环境错误（库缺失 / 非本应用库 / 损坏）
USAGE
}

# ---------- 参数解析 ----------

is_pos_int() { case "$1" in '' | *[!0-9]*) return 1 ;; esac; [ "$1" -gt 0 ]; }

# 可选值参数：下一个 token 不以 -- 开头则消费之
take_opt_value() { # $1 = 当前下标（全局 $# 语境下用位置参数）；echo 值，无则空
	if [ "$#" -ge 2 ] && [ "${2#--}" = "$2" ]; then
		printf '%s' "$2"
	fi
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--help | -h)
		usage
		exit 0
		;;
	--db)
		[ "$#" -ge 2 ] || {
			echo "✗ --db 需要一个路径参数"
			exit 1
		}
		DB="$2"
		shift 2
		;;
	--days)
		[ "$#" -ge 2 ] || {
			echo "✗ --days 需要一个正整数参数"
			exit 1
		}
		is_pos_int "$2" || {
			echo "✗ --days 参数非法: '$2'（须为正整数）"
			exit 1
		}
		DAYS="$2"
		shift 2
		;;
	--change)
		[ "$#" -ge 2 ] || {
			echo "✗ --change 需要一个 change 名参数"
			exit 1
		}
		CHANGE="$2"
		shift 2
		;;
	--lifecycle)
		[ "$#" -ge 2 ] || {
			echo "✗ --lifecycle 需要一个 change 名参数"
			exit 1
		}
		LIFECYCLE="$2"
		shift 2
		;;
	--warn-threshold)
		[ "$#" -ge 2 ] || {
			echo "✗ --warn-threshold 需要一个正整数参数"
			exit 1
		}
		is_pos_int "$2" || {
			echo "✗ --warn-threshold 参数非法: '$2'（须为正整数；0 会退化为「全部命中」，故拒绝）"
			exit 1
		}
		WARN_THRESHOLD="$2"
		shift 2
		;;
	--json)
		JSON_MODE=1
		shift
		;;
	--save-baseline)
		_opt_val="$(take_opt_value "$@")"
		if [ -n "$_opt_val" ]; then
			SAVE_BASELINE="$_opt_val"
			shift
		else
			SAVE_BASELINE="$BASELINE_DEFAULT"
		fi
		shift
		;;
	--baseline)
		_opt_val="$(take_opt_value "$@")"
		if [ -n "$_opt_val" ]; then
			BASELINE="$_opt_val"
			shift
		else
			BASELINE="$BASELINE_DEFAULT"
		fi
		shift
		;;
	*)
		echo "✗ 未知参数: '$1'"
		echo
		usage
		exit 1
		;;
	esac
done

# --lifecycle 互斥校验（体检卡是独立模式，不做七段过滤/基线比对）
if [ -n "$LIFECYCLE" ]; then
	if [ -n "$CHANGE" ]; then
		echo "✗ --lifecycle 与 --change 互斥（体检卡自带 change 维度）"
		exit 1
	fi
	if [ -n "$SAVE_BASELINE" ] || [ -n "$BASELINE" ]; then
		echo "✗ --lifecycle 不支持基线选项（体检卡无 metrics 基线语义）"
		exit 1
	fi
fi

# ---------- 库校验（只读打开 + 身份校验） ----------

if [ ! -f "$DB" ]; then
	echo "✗ 账本不存在: $DB"
	echo "  （缺省路径为 .pi/harness/events.db；本脚本只读，不会创建账本）"
	exit 1
fi

OPEN_MODE="readonly"
if ! APP_ID_RAW="$(sqlite3 -readonly "$DB" "PRAGMA application_id;" 2>/dev/null)"; then
	# WAL 库在只读打开失败时的回退（可能少最后几条未 checkpoint 的事件，报告头会标注）
	if APP_ID_RAW="$(sqlite3 -readonly -uri "file:${DB}?immutable=1" "PRAGMA application_id;" 2>/dev/null)"; then
		OPEN_MODE="immutable"
	else
		echo "✗ 无法只读打开账本: $DB（文件损坏或权限不足）"
		exit 1
	fi
fi
if [ "$APP_ID_RAW" != "$APP_ID_DEC" ]; then
	echo "✗ 拒绝读取: $DB 不是本应用的账本（application_id=$APP_ID_RAW，期望 $APP_ID_DEC / 0x53594E54）"
	exit 1
fi

run_sql() {
	if [ "$OPEN_MODE" = "immutable" ]; then
		sqlite3 -readonly -uri "file:${DB}?immutable=1"
	else
		sqlite3 -readonly "$DB"
	fi
}

# ---------- 变量装配 ----------

# SQLite 字符串字面量转义（'' 表示单引号）；库只读打开，注入亦无法写入（双重兜底）
sql_quote() {
	local s="$1"
	s="${s//\'/\'\'}"
	printf "'%s'" "$s"
}

if [ -n "$CHANGE" ]; then
	CHANGE_PRED="change = $(sql_quote "$CHANGE")"
else
	CHANGE_PRED="1=1"
fi

# 注入面健康：候选文档集取自 docs/reference/constraints-index.md 的 markdown 链接
# （该索引是「哪些文档承载约束」的权威清单；未列入的文档不参与零命中判定）
CANDS_JSON="[]"
IDX="docs/reference/constraints-index.md"
if [ -f "$IDX" ]; then
	cands="$(grep -oE '\]\([^)]+\.md\)' "$IDX" | sed -E 's/^\]\(//; s/\)$//' | while IFS= read -r t; do
		case "$t" in
		../../*) printf '%s\n' "${t#../../}" ;;
		../*) printf '%s\n' "${t#../}" ;;
		/*) printf '%s\n' "${t#/}" ;;
		docs/*) printf '%s\n' "$t" ;;
		*) printf '%s\n' "docs/reference/$t" ;;
		esac
	done | sort -u)"
	if [ -n "$cands" ]; then
		CANDS_JSON="$(printf '%s\n' "$cands" | python3 -c 'import json,sys; print(json.dumps([l.strip() for l in sys.stdin if l.strip()]))')"
	fi
fi
# JSON 文本作为 SQL 字符串字面量传入（json_each 需要文本参数）
CANDS_LIT="'$(printf '%s' "$CANDS_JSON" | sed "s/'/''/g")'"

# 域映射（⑦段 A 组注入命中率）：docs/reference/flow/*.md 头部 doc-impact-applies 标签
# （形如 <!-- doc-impact-applies: backend-go/internal/topicgraph/, ... | section=... -->）
# → [{domain, prefix}]；解析失败/无标签 → 空数组，报告降级标注「域映射不可用」
DOM_JSON="[]"
if compgen -G "docs/reference/flow/*.md" >/dev/null; then
	_dom_tmp="$(python3 - <<'DOMPY'
import re, json, glob, os
pairs = []
for f in sorted(glob.glob('docs/reference/flow/*.md')):
    dom = os.path.basename(f)[:-3]
    try:
        head = open(f, encoding='utf-8').read(4096)
    except OSError:
        continue
    m = re.search(r'doc-impact-applies:\s*([^|>]+)', head)
    if not m:
        continue
    for pfx in m.group(1).split(','):
        pfx = pfx.strip()
        if pfx:
            pairs.append({'domain': dom, 'prefix': pfx})
print(json.dumps(pairs, ensure_ascii=False))
DOMPY
	)"
	case "$_dom_tmp" in
	'['*']') DOM_JSON="$_dom_tmp" ;;
	esac
fi
DOM_LIT="'$(printf '%s' "$DOM_JSON" | sed "s/'/''/g")'"

# B' 每任务成本（task-cost-metrics）：归档目录锚点（双锚点之二）——目录名日期前缀在窗口内
# 则剥前缀得 change 名，同时读 proposal.md 头 complexity 注释分桶。环境变量
# HARNESS_RETRO_ARCHIVE_DIR 供 smoke 注入 fixture 归档目录（缺省真实 openspec/changes/archive）。
ARCH_JSON="[]"
ARCH_DIR="${HARNESS_RETRO_ARCHIVE_DIR:-openspec/changes/archive}"
SINCE_DATE="$(date -u -d "-${DAYS} day" +%Y-%m-%d)"
if [ -d "$ARCH_DIR" ]; then
	arch_tmp="$(python3 - "$ARCH_DIR" "$SINCE_DATE" <<'ARCHPY'
import json, os, re, sys
root, since = sys.argv[1], sys.argv[2]
out = []
try:
    entries = sorted(os.listdir(root))
except OSError:
    entries = []
for e in entries:
    m = re.match(r'^(\d{4}-\d{2}-\d{2})-(.+)$', e)
    if not m or m.group(1) < since:
        continue
    name = m.group(2)
    cx = None
    try:
        with open(os.path.join(root, e, 'proposal.md'), encoding='utf-8') as fh:
            head = fh.read(4096)
        cm = re.search(r'<!--\s*complexity:\s*([A-Za-z0-9_-]+)\s*-->', head)
        if cm:
            cx = cm.group(1)
    except OSError:
        pass
    out.append({'name': name, 'date': m.group(1), 'cx': cx})
print(json.dumps(out, ensure_ascii=False))
ARCHPY
	)"
	case "$arch_tmp" in
	'['*']') ARCH_JSON="$arch_tmp" ;;
	esac
fi
ARCH_LIT="'$(printf '%s' "$ARCH_JSON" | sed "s/'/''/g")'"

# 测试欠账巡检（test-debt-patrol）：test_debt 表可能不存在（老库/纯净库）或列结构变动——
# 只读 SQL 引用缺表会让整段统计报错，故先用 pragma_table_info 探列，再决定用真表还是占位常量（降级不报错）。
PATROL_DEBT_OK=0
_TD_COLS="$(printf '%s\n' "SELECT name FROM pragma_table_info('test_debt');" | run_sql 2>/dev/null || true)"
if [ -n "$_TD_COLS" ]; then
	PATROL_DEBT_OK=1
	for _c in status first_seen last_seen; do
		printf '%s\n' "$_TD_COLS" | grep -qx "$_c" || PATROL_DEBT_OK=0
	done
fi
if [ "$PATROL_DEBT_OK" = 1 ]; then
	PATROL_DEBT_CTE="debt_status AS (SELECT status, COUNT(*) AS n FROM test_debt GROUP BY status),
debt_new AS (SELECT COUNT(*) AS n FROM test_debt WHERE date(first_seen) >= date('now','-${DAYS} day')),
debt_fix AS (SELECT COUNT(*) AS n FROM test_debt WHERE status='fixed' AND date(last_seen) >= date('now','-${DAYS} day')),"
else
	PATROL_DEBT_CTE="debt_status AS (SELECT NULL AS status, 0 AS n WHERE 0),
debt_new AS (SELECT 0 AS n),
debt_fix AS (SELECT 0 AS n),"
fi

WARNINGS=""
add_warning() { WARNINGS="${WARNINGS}${WARNINGS:+
}$1"; }

if [ "$DAYS" -gt "$TTL_DAYS_MIN" ]; then
	add_warning "窗口 ${DAYS} 天 > 被消费事件类型的最短保留期 ${TTL_DAYS_MIN} 天：该窗口可能已被 TTL 清扫，与基线的差值可能失真（数据被清扫不等于指标改善）"
fi
if [ "$OPEN_MODE" = "immutable" ]; then
	add_warning "账本以 immutable=1 回退方式打开（只读打开失败）：可能少最后几条未 checkpoint 的事件"
fi

# ---------- --lifecycle 模式（add-change-lifecycle-retro design D1/D6）：单 change 体检卡 ----------

if [ -n "$LIFECYCLE" ]; then
	NAME_LIT="$(sql_quote "$LIFECYCLE")"
	# proposal 声明元数据源解析：未归档读 changes/，已归档读 archive/（目录名日期前缀剥离后拼路径）。
	# HARNESS_RETRO_CHANGES_DIR / HARNESS_RETRO_ARCHIVE_DIR 供 smoke 注入 fixture 目录。
	LIFE_CHANGES_DIR="${HARNESS_RETRO_CHANGES_DIR:-openspec/changes}"
	LIFE_META_PATH=""
	LIFE_DIR_HIT=0
	if [ -f "$LIFE_CHANGES_DIR/$LIFECYCLE/proposal.md" ]; then
		LIFE_META_PATH="$LIFE_CHANGES_DIR/$LIFECYCLE/proposal.md"
	elif LIFE_DIR_HIT_PATH="$(compgen -G "$ARCH_DIR/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-$LIFECYCLE" 2>/dev/null | head -n 1)" && [ -n "${LIFE_DIR_HIT_PATH:-}" ]; then
		LIFE_META_PATH="$LIFE_DIR_HIT_PATH/proposal.md"
		LIFE_DIR_HIT=1
	fi
	# 第五段人类反馈文件目录（design D7/D8）：路径规则同声明元数据——未归档读 changes/、
	# 已归档读 archive/ 剥日期前缀；目录判定独立于 proposal.md 存在性（human-feedback.md
	# 可独立存在），优先级一致：changes/ 未归档目录优先。
	LIFE_CHANGE_DIR=""
	if [ -d "$LIFE_CHANGES_DIR/$LIFECYCLE" ]; then
		LIFE_CHANGE_DIR="$LIFE_CHANGES_DIR/$LIFECYCLE"
	elif [ "$LIFE_DIR_HIT" = "1" ]; then
		LIFE_CHANGE_DIR="$LIFE_DIR_HIT_PATH"
	fi

	LIFE_SQL="$(cat <<LIFESQL
WITH ev AS (
  SELECT id, ts, session_id, kind, payload FROM events WHERE change = ${NAME_LIT}
),
term AS (
  SELECT
    (SELECT MIN(ts) FROM events WHERE kind='change.archive' AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) = ${NAME_LIT}) AS arch_ts,
    (SELECT COUNT(*) FROM events WHERE kind='change.archive' AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) = ${NAME_LIT}) AS arch_n,
    (SELECT MIN(ts) FROM events WHERE kind='change.abandon' AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) = ${NAME_LIT}) AS abandon_ts,
    (SELECT COUNT(*) FROM events WHERE kind='change.abandon' AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) = ${NAME_LIT}) AS abandon_n
),
sess AS (SELECT COUNT(DISTINCT session_id) n, MAX(ts) last_ts FROM ev),
mode_seq AS (SELECT ts, session_id, json_extract(payload,'$.mode') m, json_extract(payload,'$.source') src FROM ev WHERE kind='mode.set' ORDER BY id),
mode_by AS (SELECT json_extract(payload,'$.mode') m, COUNT(*) n FROM ev WHERE kind='mode.set' GROUP BY 1),
inj AS (SELECT COALESCE(json_extract(payload,'$.reason'),'(无 reason)') reason, COUNT(*) n, IFNULL(SUM(CAST(json_extract(payload,'$.bytes') AS INTEGER)),0) b FROM ev WHERE kind='constraint.inject' GROUP BY 1 ORDER BY n DESC),
pinw AS (SELECT COUNT(*) n FROM ev WHERE kind='pin.write'),
pinr AS (SELECT COUNT(*) n FROM ev WHERE kind='pin.read'),
emap_last AS (SELECT CAST(json_extract(payload,'$.n') AS INTEGER) n, ts FROM ev WHERE kind='edit.map' ORDER BY id DESC LIMIT 1),
emap_snaps AS (SELECT COUNT(*) n FROM ev WHERE kind='edit.map'),
emap_flat AS (SELECT session_id, je.value p FROM ev, json_each(json_extract(payload,'$.paths')) je WHERE kind='edit.map' AND json_valid(payload)),
wave AS (SELECT p, COUNT(DISTINCT session_id) n FROM emap_flat GROUP BY 1 HAVING n >= 2 ORDER BY n DESC, p LIMIT 10),
g AS (
  SELECT id, ts, session_id,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.ok') END ok,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.cmd') END cmd,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.sampled') END sampled,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.n') END n
  FROM ev WHERE kind='gate.check'
),
gc AS (SELECT * FROM g WHERE ok IS NOT NULL),
filled AS (SELECT *, CASE WHEN ok=0 THEN 0 WHEN sampled=1 THEN (CASE WHEN n IS NULL THEN ${DEFAULT_SAMPLE_N} WHEN typeof(n)='integer' AND n > 0 THEN n ELSE 1 END) ELSE 1 END AS ok_runs FROM gc),
gcmd AS (SELECT IFNULL(cmd,'(无 cmd)') cmd, IFNULL(SUM(ok_runs),0) runs, SUM(CASE WHEN ok=0 THEN 1 ELSE 0 END) fails FROM filled GROUP BY 1 ORDER BY fails DESC, runs DESC LIMIT 8),
gsum AS (SELECT IFNULL(SUM(ok_runs),0) runs, SUM(CASE WHEN ok=0 THEN 1 ELSE 0 END) fails FROM filled),
reg AS (SELECT COUNT(*) n FROM (SELECT id, ok, MAX(CASE WHEN ok=1 THEN id END) OVER (PARTITION BY session_id, cmd ORDER BY id ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING) prev_ok FROM gc) WHERE ok=0 AND prev_ok IS NOT NULL),
uip AS (SELECT id, ts, json_extract(payload,'$.action') action, json_extract(payload,'$.reasonCode') reason FROM ev WHERE kind='policy.decision' AND json_valid(payload) AND json_extract(payload,'$.policy')='ui-design-gate' ORDER BY id),
ui_block AS (SELECT reason, COUNT(*) n FROM uip WHERE action='block' GROUP BY 1 ORDER BY n DESC),
ui_pass AS (SELECT ts, reason FROM uip WHERE action='pass' ORDER BY id),
ui_last AS (SELECT id, ts, reason FROM uip WHERE action='block' ORDER BY id DESC LIMIT 1),
ru_raw AS (SELECT id, session_id, change, payload FROM events WHERE kind='session.rollup' AND json_valid(payload)),
ru AS (SELECT * FROM ru_raw WHERE id IN (SELECT MAX(id) FROM ru_raw GROUP BY session_id, change)), -- 探索验证配方（design D1）：终值按 (session_id, change) 取 MAX——交织会话按绑定段归因（与 B' 按会话终归的既有口径并存，体检卡以此为准）
ru_px AS (SELECT ru.id, ru.session_id, COALESCE(ru.change,'') chg,
    CAST(json_extract(payload,'$.turns') AS INTEGER) turns,
    CAST(json_extract(payload,'$.toolCalls') AS INTEGER) tc,
    CAST(json_extract(payload,'$.cost') AS REAL) cost,
    CAST(json_extract(payload,'$.durationSec') AS INTEGER) dur,
    json_extract(payload,'$.parentSessionId') parent,
    CASE WHEN json_valid(json_extract(payload,'$.models')) THEN json_extract(payload,'$.models') END models_json
  FROM ru),
ru_attr AS (SELECT p.session_id, p.turns, p.tc, p.cost, p.dur, p.models_json,
    CASE WHEN p.chg != '' THEN p.chg WHEN p.parent IS NOT NULL THEN (SELECT f.chg FROM ru_px f WHERE f.session_id=p.parent AND f.chg!='' ORDER BY f.id DESC LIMIT 1) END AS attr
  FROM ru_px p),
life_ru AS (SELECT * FROM ru_attr WHERE attr = ${NAME_LIT}),
cost_sum AS (SELECT COUNT(*) n, IFNULL(SUM(cost),0) cost, IFNULL(SUM(turns),0) turns, IFNULL(SUM(tc),0) tc, IFNULL(SUM(dur),0) dur FROM life_ru),
cost_model AS (SELECT je.key model, SUM(CAST(json_extract(je.value,'$.cost') AS REAL)) cost FROM life_ru, json_each(life_ru.models_json) je GROUP BY 1 ORDER BY 2 DESC),
cost_nom AS (SELECT IFNULL(SUM(cost),0) c FROM life_ru WHERE models_json IS NULL),
chg_sess AS (SELECT DISTINCT session_id FROM ev),
disp_e AS (SELECT d.id, d.ts, d.session_id FROM events d JOIN chg_sess s ON d.session_id = s.session_id
  WHERE d.kind='subagent.dispatch' AND json_valid(d.payload) AND NOT COALESCE(json_extract(d.payload,'$.isError'),0)),
arch_e AS (SELECT id, ts FROM events WHERE kind='change.archive' AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) = ${NAME_LIT}),
pb AS (SELECT id, ts, session_id, json_extract(payload,'$.policy') policy FROM ev WHERE kind='policy.decision' AND json_valid(payload)
  AND json_extract(payload,'$.action') IN ('block','warn')
  AND json_extract(payload,'$.policy') IN ('ui-design-gate','quota-gate','spec-gate')),
uipass AS (SELECT id, ts FROM uip WHERE action='pass'),
pr AS (SELECT b.policy,
    CASE b.policy
      WHEN 'ui-design-gate' THEN (SELECT MIN((julianday(p.ts)-julianday(b.ts))*86400.0) FROM uipass p WHERE p.id > b.id)
      WHEN 'quota-gate' THEN (SELECT MIN((julianday(d.ts)-julianday(b.ts))*86400.0) FROM disp_e d WHERE d.id > b.id AND d.session_id = b.session_id)
      WHEN 'spec-gate' THEN (SELECT MIN((julianday(a.ts)-julianday(b.ts))*86400.0) FROM arch_e a WHERE a.id > b.id)
    END sec
  FROM pb b),
repair_p AS (SELECT policy, sec FROM pr WHERE sec IS NOT NULL AND sec >= 0),
repair_x AS (SELECT COUNT(*) n FROM pr WHERE sec IS NULL OR sec < 0),
gseq AS (SELECT ts, cmd, ok, LEAD(ok) OVER (PARTITION BY session_id, cmd ORDER BY id) nok, LEAD(ts) OVER (PARTITION BY session_id, cmd ORDER BY id) nts FROM gc),
repair_gate AS (SELECT (julianday(nts)-julianday(ts))*86400.0 sec FROM gseq WHERE ok=0 AND nok=1),
repair_all AS (SELECT sec FROM repair_gate WHERE sec IS NOT NULL AND sec >= 0 UNION ALL SELECT sec FROM repair_p),
q_rep AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT sec v FROM repair_all)),
/* review 覆盖段（add-review-trace-gate design D7）：dispatch 分类参考口径——desc 含
   「修复」优先归修复类（防「修复 review M1」误计 review 类）；仅有带 change 归属的
   dispatch 才在 ev 内（历史无归属行不回填；total=0 时展示层标注「归属改造前无数据」） */
rv_disp AS (SELECT json_extract(payload,'$.type') typ, json_extract(payload,'$.desc') dsc
  FROM ev WHERE kind='subagent.dispatch' AND json_valid(payload)),
rv_cls AS (SELECT CASE WHEN dsc IS NOT NULL AND dsc LIKE '%修复%' THEN 'repair'
    WHEN typ = 'change-review-glm' OR (dsc LIKE '%review%' OR dsc LIKE '%审查%') THEN 'review'
    ELSE 'other' END cls FROM rv_disp)
SELECT json_object(
  'name', ${NAME_LIT},
  'terminal', json_object('arch_ts',(SELECT arch_ts FROM term),'arch_n',(SELECT arch_n FROM term),'abandon_ts',(SELECT abandon_ts FROM term),'abandon_n',(SELECT abandon_n FROM term)),
  'sessions', json_object('total',(SELECT n FROM sess),'last_activity',(SELECT last_ts FROM sess)),
  'flow', json_object(
    'mode_seq', (SELECT json_group_array(json_object('ts',ts,'session',session_id,'mode',m,'source',src)) FROM mode_seq),
    'mode_by', (SELECT json_group_array(json_object('mode',m,'n',n)) FROM mode_by),
    'inject', (SELECT json_group_array(json_object('reason',reason,'n',n,'bytes',b)) FROM inj),
    'pin', json_object('write',(SELECT n FROM pinw),'read',(SELECT n FROM pinr)),
    'emap', json_object('paths',(SELECT n FROM emap_last),'snaps',(SELECT n FROM emap_snaps),'last_ts',(SELECT ts FROM emap_last)),
    'gate', json_object('runs',(SELECT runs FROM gsum),'fails',(SELECT fails FROM gsum),'by_cmd',(SELECT json_group_array(json_object('cmd',cmd,'runs',runs,'fails',fails)) FROM gcmd))
  ),
  'ui', json_object(
    'blocks', (SELECT json_group_array(json_object('reason',reason,'n',n)) FROM ui_block),
    'passes', (SELECT json_group_array(json_object('ts',ts,'reason',reason)) FROM ui_pass),
    'last_block', (SELECT json_object('ts',ts,'reason',reason) FROM ui_last)
  ),
  'cost', json_object(
    'sessions',(SELECT n FROM cost_sum),'cost',(SELECT cost FROM cost_sum),'turns',(SELECT turns FROM cost_sum),'toolCalls',(SELECT tc FROM cost_sum),'durationSec',(SELECT dur FROM cost_sum),
    'by_model',(SELECT json_group_array(json_object('model',model,'cost',cost)) FROM cost_model),
    'unmodel_cost',(SELECT c FROM cost_nom)
  ),
  'health', json_object(
    'repair', json_object('median_sec',(SELECT v FROM q_rep WHERE rn=(n+1)/2),'samples',(SELECT n FROM q_rep),'excluded',(SELECT IFNULL(n,0) FROM repair_x)),
    'regressions',(SELECT n FROM reg),
    'waves',(SELECT json_group_array(json_object('path',p,'sessions',n)) FROM wave)
  ),
  'review', json_object(
    'dispatch_total',(SELECT COUNT(*) FROM rv_disp),
    'review_disp',(SELECT COUNT(*) FROM rv_cls WHERE cls='review'),
    'repair_disp',(SELECT COUNT(*) FROM rv_cls WHERE cls='repair')
  )
);
LIFESQL
	)"

	if ! LIFE_STATS="$(printf '%s\n' "$LIFE_SQL" | run_sql 2>&1)"; then
		echo "✗ 体检卡查询失败: $DB"
		printf '%s\n' "$LIFE_STATS" | sed 's/^/  /'
		exit 1
	fi
	case "$LIFE_STATS" in
	'{'*) ;;
	*)
		echo "✗ 体检卡查询未返回预期结果: $DB"
		printf '%s\n' "$LIFE_STATS" | sed 's/^/  /'
		exit 1
		;;
	esac

	LIFE_TMP="$(mktemp)"
	trap 'rm -f "$LIFE_TMP"' EXIT
	printf '%s' "$LIFE_STATS" >"$LIFE_TMP"

	python3 - "$LIFE_TMP" "$JSON_MODE" "$DB" "$LIFECYCLE" "$LIFE_META_PATH" "$LIFE_DIR_HIT" \
		"$OPEN_MODE" "$WARNINGS" "$ARCH_DIR" "$LIFE_CHANGE_DIR" <<'LIFEPY'
import json, os, re, sys
import unicodedata


def wide(s):
    return sum(2 if unicodedata.east_asian_width(ch) in ('W', 'F') else 1 for ch in (s or ''))


def pad(s, width):
    s = s or ''
    return s + ' ' * max(0, width - wide(s))


def _pct(num, den):
    if not den:
        return None
    return int((num * 200 + den) // (2 * den))

(stats_path, json_mode, db, name, meta_path, dir_hit, open_mode,
 warnings_raw, arch_dir, fb_dir) = sys.argv[1:11]
dir_hit = dir_hit == '1'

with open(stats_path, encoding='utf-8') as fh:
    st = json.load(fh)

warnings = [w for w in (warnings_raw or '').split('\n') if w.strip()]
term = st.get('terminal') or {}
sess = st.get('sessions') or {}
flow = st.get('flow') or {}
ui = st.get('ui') or {}
cost = st.get('cost') or {}
health = st.get('health') or {}

# ---- 声明元数据（proposal 头注释，账本外的声明侧；缺失标注「声明缺席」） ----
meta = {'ui_impact': None, 'complexity': None, 'constraint_domains': None, 'source': meta_path or None}
if meta_path:
    try:
        with open(meta_path, encoding='utf-8') as fh:
            head = fh.read(4096)
        for key, marker in (('ui_impact', 'ui-impact'), ('complexity', 'complexity'), ('constraint_domains', 'constraint-domains')):
            m = re.search(r'<!--\s*%s\s*:\s*([^>]*?)\s*-->' % marker, head)
            meta[key] = m.group(1) if m else None
    except OSError as exc:
        warnings.append('proposal 元数据读取失败（%s）：%s' % (meta_path, exc))

# ---- 人类反馈段数据源（design D7/D8：文件为源，路径规则同声明元数据；
#      宽容降级：单条解析失败计「格式异常」单列披露，不炸整段） ----
FB_STATUSES = ('待处理', '处理中', '已消化', '已转挂', '已放弃')

def parse_feedback(change_dir):
    '''返回 (stat dict or None, error str or None)；文件缺席/无条目 → (None, None)。'''
    if not change_dir:
        return None, None
    path = change_dir.rstrip('/') + '/human-feedback.md'
    if not os.path.isfile(path):
        return None, None
    try:
        with open(path, encoding='utf-8') as fh:
            content = fh.read()
    except OSError as exc:
        return None, '读取失败（%s）：%s' % (path, exc)
    stat = {'total': 0, 'types': {}, 'layers': {}, 'open': 0, 'open_ids': [], 'bad': 0, 'bad_ids': []}
    for blk in re.split(r'(?m)^(?=### FB-\d+)', content):
        if not re.match(r'^### FB-\d+', blk):
            continue
        stat['total'] += 1
        head = blk.split('\n', 1)[0]
        no = re.match(r'^### FB-(\d+)', head).group(1)
        hm = re.match(r'^### FB-\d+\s+\S+【(产品|流程)\|(frontend|backend|test-gap|fact-check|process)】', head)
        sm = re.search(r'(?m)^- \*\*状态\*\*：\s*(.*)$', blk)
        st_word = next((w for w in FB_STATUSES if sm and sm.group(1).strip().startswith(w)), None)
        if not hm or st_word is None:
            stat['bad'] += 1
            stat['bad_ids'].append(no)
            continue
        stat['types'][hm.group(1)] = stat['types'].get(hm.group(1), 0) + 1
        stat['layers'][hm.group(2)] = stat['layers'].get(hm.group(2), 0) + 1
        if st_word in ('待处理', '处理中'):
            stat['open'] += 1
            stat['open_ids'].append('FB-%s' % no)
    if not stat['total']:
        return None, None  # 文件存在但无条目 → 同缺席降级（spec：无条目输出降级标注）
    return stat, None

fb, fb_err = parse_feedback(fb_dir)

# ---- review 覆盖段数据源（add-review-trace-gate）：文件痕迹三态（权威口径）——
#      review.md 建账 / proposal 头部 waived 声明 / 皆无；判据与 review-trace.sh 同构
#      （一行正则全解析，标题与状态行间允许空行；waived 优先，双痕迹展示并存） ----
def parse_review_trace(change_dir, meta_path):
    '''返回 {'waived','entries','no_finding'}；文件缺席按零痕迹降级（不炸段）'''
    out_rv = {'waived': False, 'entries': 0, 'no_finding': False}
    if meta_path:
        try:
            with open(meta_path, encoding='utf-8') as fh:
                for line in fh:
                    s = line.strip()
                    if not s:
                        continue
                    if not (s.startswith('<!--') and s.endswith('-->')):
                        break  # 头部机器可读注释区结束（正文段落 marker 不识别）
                    if re.match(r'^<!--\s*review\s*:\s*waived\s*-->$', s):
                        out_rv['waived'] = True
        except OSError:
            pass
    if change_dir:
        path = change_dir.rstrip('/') + '/review.md'
        if os.path.isfile(path):
            try:
                with open(path, encoding='utf-8') as fh:
                    content = fh.read()
                pending = False
                for line in content.split('\n'):
                    if re.match(r'^### R-\d+ \d{4}-\d{2}-\d{2}【(High|Medium|Low)】.+', line):
                        pending = True
                        continue
                    if pending:
                        if re.match(r'^- \*\*状态\*\*：(待处理|已修复|已采纳|已驳回|误报丢弃)(（.*）)?\s*$', line):
                            out_rv['entries'] += 1
                            pending = False
                        elif not line.strip():
                            pass  # 标题与状态行间的惯例空行
                        else:
                            pending = False
                out_rv['no_finding'] = bool(re.search(r'(?m)^- 审毕无发现（审查范围：.+）\s*$', content))
            except OSError:
                pass
    return out_rv

rv_trace = parse_review_trace(fb_dir, meta_path)

# ---- 终态判定（archive 优先；abandon×archive 并存冲突提示；未归档不假定终态） ----
archived = term.get('arch_ts') is not None or dir_hit
abandoned = term.get('abandon_ts') is not None
conflict = archived and abandoned
if conflict:
    terminal_txt = '已归档（change.archive @ %s）' % term.get('arch_ts')
elif archived:
    terminal_txt = '已归档（%s）' % (
        ('change.archive @ %s' % term.get('arch_ts')) if term.get('arch_ts')
        else 'archive 目录锚点（事件缺记或已过保留期，目录兑底）')
elif abandoned:
    terminal_txt = '已放弃（change.abandon @ %s，不假定会归档）' % term.get('abandon_ts')
else:
    terminal_txt = '未归档（最后活动 %s；不假定终态——活跃/放弃需后续事实）' % (sess.get('last_activity') or '无事件')

empty = (sess.get('total') or 0) == 0 and not archived and not abandoned

if json_mode == '1':
    doc = {
        'meta': {
            'change': name, 'db': db, 'open_mode': open_mode,
            'generated_at': __import__('datetime').datetime.now(__import__('datetime').timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
            'declaration': meta, 'dir_anchor': dir_hit, 'empty': empty,
        },
        'terminal': {'state': ('conflict-archived' if conflict else 'archived' if archived else 'abandoned' if abandoned else 'unarchived'),
                     'arch_ts': term.get('arch_ts'), 'abandon_ts': term.get('abandon_ts'), 'conflict': conflict},
        'sessions': sess, 'flow': flow, 'ui': ui, 'cost': cost, 'health': health,
        'human_feedback': {'present': fb is not None, 'error': fb_err, **(fb or {})},
        'review': {'state': ('waived' if rv_trace['waived'] else 'review_md' if rv_trace['entries'] else 'no_finding' if rv_trace['no_finding'] else 'none'),
                   **rv_trace, 'archived': archived, 'dispatch': st.get('review') or {}},
        'warnings': warnings,
    }
    json.dump(doc, sys.stdout, ensure_ascii=False, indent=2, sort_keys=True)
    sys.stdout.write('\n')
    sys.exit(0)

# ---- 文本渲染 ----
out = []
out.append('harness-retro --lifecycle — 单 change 生命周期体检卡')
out.append('账本      : %s（%s 打开，只读；全程零写入）' % (db, open_mode))
out.append('Change    : %s' % name)
if meta['source']:
    out.append('声明元数据: ui-impact=%s / complexity=%s / constraint-domains=%s（源：%s）' % (
        meta['ui_impact'] or '（未声明）', meta['complexity'] or '（未声明）',
        meta['constraint_domains'] or '（未声明）', meta['source']))
else:
    out.append('声明元数据: 声明缺席（changes/ 与 archive/ 下均未找到 proposal.md）')
out.append('终态      : %s' % terminal_txt)
if conflict:
    out.append('冲突提示  : 同名 change.archive 与 change.abandon 并存（abandon @ %s）——以归档为准，abandon 疑为误登记' % term.get('abandon_ts'))
out.append('')

if empty:
    out.append('该 change 名下无任何事件且无终态锚点：各段降级为缺席标注（可能从未启动，或事件已过 TTL 30 天保留期）。')
else:
    # ---- 一、流程规范段 ----
    out.append('一、流程规范段（档位绑定 → 约束注入 → pin 接力 → 编辑归属 → 门禁执行 → 终态锚点）')
    ms = flow.get('mode_seq') or []
    if ms:
        out.append('   档位绑定        : %d 次 mode.set（%s）' % (
            len(ms),
            ' / '.join('%s@%s' % ((it.get('mode') or '?'), (it.get('ts') or '')[5:16]) for it in ms[:6]) + (' …' if len(ms) > 6 else '')))
    else:
        out.append('   档位绑定        : 无 mode.set 记录（requirements 档未激活，或已过保留期）')
    inj = flow.get('inject') or []
    if inj:
        _n = sum(it['n'] for it in inj)
        _b = sum(it.get('bytes') or 0 for it in inj)
        out.append('   约束注入        : %d 条 / %d B（%s）' % (_n, _b, ' / '.join('%s×%s' % (it['reason'], it['n']) for it in inj[:5])))
    else:
        out.append('   约束注入        : 无 constraint.inject 记录')
    pin = flow.get('pin') or {}
    out.append('   pin 接力        : write %s 条 / read %s 条' % (pin.get('write') or 0, pin.get('read') or 0))
    em = flow.get('emap') or {}
    if em.get('snaps'):
        out.append('   编辑归属        : %s 个路径（edit.map 快照 %s 次，最新 @ %s）' % (em.get('paths'), em.get('snaps'), (em.get('last_ts') or '')[:19]))
    else:
        out.append('   编辑归属        : 无 edit.map 记录（未发生结构化编辑，或已过保留期）')
    gt = flow.get('gate') or {}
    if (gt.get('runs') or 0) + (gt.get('fails') or 0) > 0:
        out.append('   门禁执行        : %s 次执行 / %s 次失败（还原口径；Top：%s）' % (
            gt.get('runs') or 0, gt.get('fails') or 0,
            '；'.join('%s %s/%s' % ((it['cmd'] or '?')[:36], it['runs'], it['fails']) for it in (gt.get('by_cmd') or [])[:4]) or '无'))
    else:
        out.append('   门禁执行        : 无 gate.check 记录')
    out.append('   终态锚点        : %s' % terminal_txt)
    out.append('')

    # ---- 二、UI 工作流段 ----
    out.append('二、UI 工作流段（ui-design-gate 卡点链 × 放行态）')
    blocks = ui.get('blocks') or []
    passes = ui.get('passes') or []
    if blocks:
        out.append('   卡点链          : %s（block 计数）' % ' / '.join('%s ×%s' % (it['reason'], it['n']) for it in blocks))
    else:
        out.append('   卡点链          : 无 ui-design-gate block 记录%s' % ('（有 pass 放行锚点）' if passes else ''))
    if passes:
        out.append('   放行态          : 已放行（%s）' % '；'.join('pass %s @ %s' % (it['reason'], (it['ts'] or '')[:19]) for it in passes[:4]))
    elif blocks:
        lb = ui.get('last_block') or {}
        out.append('   放行态          : 推断（无放行事件）：末次 block %s @ %s 后无新 block——引入 pass 记账（2026-09-28）前的历史只能推断' % (lb.get('reason'), (lb.get('ts') or '')[:19]))
    else:
        out.append('   放行态          : 无卡点记录（ui-impact: none/minor 直接过，或未触及前端路径）')
    out.append('')

    # ---- 三、成本卡 ----
    out.append('三、成本卡（session.rollup 终值聚合，主会话直归 + 子会话归父）')
    cs = cost.get('sessions') or 0
    st_n = sess.get('total') or 0
    if cs > 0:
        cov = _pct(cs, st_n) if st_n else None
        cov_txt = '' if (cov is None or cov >= 100) else '（覆盖率 %s/%s session%s）' % (cs, st_n, '，⚠ 数据积累中' if (cov is not None and cov < 50) else '')
        out.append('   归因会话        : %s 个 rollup 终值会话%s' % (cs, cov_txt))
        out.append('   合计            : cost ¥%.2f / turns %s / toolCalls %s / 时长 %.1f h' % (
            cost.get('cost') or 0, cost.get('turns') or 0, cost.get('toolCalls') or 0, (cost.get('durationSec') or 0) / 3600.0))
        bm = cost.get('by_model') or []
        un = cost.get('unmodel_cost')
        segs = ['%s ¥%.2f' % (pad((it.get('model') or '(无模型)')[:28], 30), it.get('cost') or 0) for it in bm]
        if un:
            segs.append('(未分模型) ¥%.2f' % un)
        out.append('   按模型分桶      : %s' % ('；'.join(segs) if segs else '—（终值均无 models 字段）'))
    else:
        out.append('   归因会话        : 无 rollup 终值（数据积累中，或该 change 早于 2026-09-17 rollup 上线——历史会话无成本维度）')
    out.append('')

    # ---- 四、健康度段 ----
    out.append('四、健康度段（催修时距 / 回归翻转 / 返工波次）')
    rep = health.get('repair') or {}
    med = rep.get('median_sec')
    if rep.get('samples'):
        out.append('   催修时距        : 中位 %s（n=%s；分派：quality-gate=转绿 / ui-design-gate=pass / quota-gate=成功派发 / spec-gate=归档%s）' % (
            '%.1f min' % (med / 60.0) if med is not None else '—', rep.get('samples'),
            '；剔除无解除信号 %s 条' % rep.get('excluded') if rep.get('excluded') else ''))
    else:
        out.append('   催修时距        : 无样本（%s）' % ('剔除 %s 条无解除信号' % rep.get('excluded') if rep.get('excluded') else '无 block/warn 或无 gate.check 红→绿'))
    out.append('   回归翻转        : %s 次（同 session+cmd 曾绿变红）' % (health.get('regressions') or 0))
    waves = health.get('waves') or []
    out.append('   返工波次        : %s 个文件 ≥2 session（累计快照口径，含被动携带）' % len(waves))
    for it in waves[:6]:
        out.append('     - %s ×%s session' % (pad((it.get('path') or '')[:56], 58), it.get('sessions')))
    out.append('')

# ---- 五、人类反馈段（design D7/D8：条数/归因分布/未处置；文件缺席降级；格式异常单列披露不炸段） ----
# 独立于事件 empty 判定：反馈文件与事件账本正交，无事件 change 也可有反馈记录。
out.append('五、人类反馈段（human-feedback.md：条目 / 归因分布 / 未处置）')
if fb_err:
    out.append('   反馈记录        : 读取失败降级——%s' % fb_err)
elif fb is None:
    out.append('   反馈记录        : 无反馈记录（或未启用）')
else:
    types_txt = ' / '.join('%s %s' % (k, v) for k, v in sorted(fb['types'].items())) or '—'
    out.append('   条目            : %s（可解析 %s/%s 条）' % (types_txt, fb['total'] - fb['bad'], fb['total']))
    layers_txt = ' / '.join('%s×%s' % (k, v) for k, v in sorted(fb['layers'].items(), key=lambda kv: (-kv[1], kv[0]))) or '—'
    out.append('   归因分布        : %s' % layers_txt)
    out.append('   未处置          : %s 条（待处理/处理中：%s）' % (fb['open'], '、'.join(fb['open_ids']) or '无'))
    if fb['bad']:
        out.append('   格式异常        : %s 条（%s）——词表外归因/缺状态行，单列披露不炸段' % (fb['bad'], '、'.join('FB-%s' % i for i in fb['bad_ids'])))
out.append('')

# ---- 六、review 覆盖段（add-review-trace-gate design D1/D7：文件痕迹三态为权威口径，
#      dispatch 分类计数为参考口径——desc 关键词有误报面，「修复」优先归修复类防误计；
#      独立于事件 empty 判定：痕迹文件与事件账本正交） ----
out.append('六、review 覆盖段（review.md 痕迹 / waived 豁免 / review 类派发参考口径）')
if rv_trace['waived']:
    _dual = ''
    if rv_trace['entries']:
        _dual = '（双痕迹：另检出 review.md 建账 %s 条，以 waived 计）' % rv_trace['entries']
    elif rv_trace['no_finding']:
        _dual = '（双痕迹：另检出审毕无发现声明，以 waived 计）'
    out.append('   痕迹形态        : waived 显式豁免（proposal 头部声明）%s' % _dual)
elif rv_trace['entries']:
    out.append('   痕迹形态        : review.md 建账（%s 条）' % rv_trace['entries'])
elif rv_trace['no_finding']:
    out.append('   痕迹形态        : 审毕无发现（review.md 显式声明）')
elif archived:
    out.append('   痕迹形态        : 皆无（已归档——缺口：归档前未做 review 也未显式豁免）')
else:
    out.append('   痕迹形态        : 皆无（未归档——归档门禁 review-trace 将对账，两者皆无则阻断）')
_rvdisp = (st.get('review') or {})
if (_rvdisp.get('dispatch_total') or 0) > 0:
    out.append('   review 类派发   : %s 次（参考口径：type=change-review-glm 或 desc 含 review/审查）' % (_rvdisp.get('review_disp') or 0))
    out.append('   修复类派发      : %s 次（参考口径：desc 含「修复」优先归类，不计 review 类）' % (_rvdisp.get('repair_disp') or 0))
else:
    out.append('   review 类派发   : 归属改造前无数据（dispatch 事件 change 归属自 add-review-trace-gate 落地起记录，历史不回填）')
out.append('')

if warnings:
    out.append('提示 / 降级:')
    for w in warnings:
        out.append('  ⚠ %s' % w)
    out.append('')

sys.stdout.write('\n'.join(out))
LIFEPY
	exit 0
fi

# ---------- 统计（单次 SQL：聚合下推到 SQLite，数值可独立复算） ----------

STATS_SQL="$(
	cat <<SQL
WITH win AS (
  SELECT id, ts, session_id, kind, change, payload
  FROM events
  WHERE ts >= strftime('%Y-%m-%dT%H:%M:%fZ','now','-${DAYS} day') AND ${CHANGE_PRED}
),
g AS (
  SELECT id, ts, session_id,
    json_valid(payload) AS valid,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.ok') END      AS ok,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.cmd') END     AS cmd,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.sampled') END AS sampled,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.n') END       AS n,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.flip') END    AS flip,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.diag') END    AS diag
  FROM win WHERE kind='gate.check'
),
gc AS (SELECT * FROM g WHERE valid=1 AND ok IS NOT NULL),
filled AS (
  SELECT *,
    CASE WHEN ok=0 THEN 0
         WHEN sampled=1 THEN
           CASE WHEN n IS NULL THEN ${DEFAULT_SAMPLE_N}
                WHEN typeof(n)='integer' AND n > 0 THEN n
                ELSE 1 END
         ELSE 1 END AS ok_runs
  FROM gc
),
agg AS (SELECT IFNULL(SUM(ok_runs),0) AS ok_runs, SUM(CASE WHEN ok=0 THEN 1 ELSE 0 END) AS fails FROM filled),
reg AS (
  SELECT id FROM (
    SELECT id, ok,
           MAX(CASE WHEN ok=1 THEN id END) OVER (
             PARTITION BY session_id, cmd ORDER BY id ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
           ) AS prev_ok
    FROM gc
  ) WHERE ok=0 AND prev_ok IS NOT NULL
),
famrows AS (
  SELECT CASE
    WHEN diag IS NULL OR trim(diag)='' THEN '无 diag（空失败输出）'
    WHEN diag LIKE '%command not found%' OR diag LIKE '%parallel golangci-lint is running%' OR diag LIKE '%text file busy%'
      OR diag LIKE '%resource temporarily unavailable%' OR diag LIKE '%no space left%'
      OR diag LIKE '%vsock%' OR diag LIKE '%vet.exe: chdir%' OR diag LIKE '%exec format error%'
      THEN '并发/环境冲突（疑非代码问题）'
    WHEN diag LIKE '%[setup failed]%' OR diag LIKE '%cannot connect to the docker daemon%'
      OR diag LIKE '%container is not running%' OR diag LIKE '%failed to start container%'
      THEN '测试环境未就绪（setup failed）'
    WHEN diag LIKE '%dial tcp%' OR diag LIKE '%no such host%' OR diag LIKE '%connection refused%'
      OR diag LIKE '%i/o timeout%' OR diag LIKE '%tls handshake%' OR diag LIKE '%proxyconnect%'
      THEN '依赖/网络'
    WHEN diag LIKE '%build failed%' OR diag LIKE '%typechecking error%' OR diag LIKE '%unknown field%'
      OR diag LIKE '%undefined:%' OR diag LIKE '%cannot use%' OR diag LIKE '%declared and not used%'
      OR diag LIKE '%syntax error%' OR diag LIKE '%no required module%' OR diag LIKE '%vet:%'
      OR diag LIKE '%missing '','' in%' OR diag LIKE '%expected operand%' OR diag LIKE '%expected '';''%'
      OR diag LIKE '%# syntopica-%' OR diag LIKE '%# command-line-arguments%'
      THEN '编译/类型/vet 语法'
    WHEN diag LIKE '%--- fail%' OR diag LIKE 'fail%' OR diag LIKE '%fail %'
      OR diag LIKE '%panic%' OR diag LIKE '%test timed out%'
      THEN '单测失败'
    WHEN diag LIKE '%.go:%:%: %' OR diag LIKE '%unused%' OR diag LIKE '%gofmt%'
      OR diag LIKE '%ineffassign%' OR diag LIKE '%commentformatting%' OR diag LIKE '%not properly formatted%'
      OR diag LIKE '%lint%'
      THEN 'lint 规则'
    ELSE '其它/未归类'
  END AS fam
  FROM gc WHERE ok=0
),
famrows2 AS ( -- 按 domain 包聚类：从命令里取 ./internal/<domain>/ 段（仓库全量命令如 lint/vet/build 无包维度，归入 NULL）
  SELECT CASE
    WHEN instr(h,'internal/') > 0 THEN
      CASE WHEN instr(substr(h, instr(h,'internal/')+9), '/') > 0
           THEN substr(h, instr(h,'internal/')+9, instr(substr(h, instr(h,'internal/')+9), '/')-1)
           ELSE substr(h, instr(h,'internal/')+9) END
    ELSE NULL
  END AS dom
  FROM (SELECT IFNULL(cmd,'') AS h FROM gc WHERE ok=0)
),
nd AS (
  SELECT id, ts, session_id, cmd, diag,
    CASE WHEN diag IS NULL OR trim(diag)='' THEN '(no diag)'
         ELSE trim(substr(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(
                substr(diag,1,200),
              '0','#'),'1','#'),'2','#'),'3','#'),'4','#'),'5','#'),'6','#'),'7','#'),'8','#'),'9','#'),1,120))
    END AS ndiag
  FROM gc WHERE ok=0
),
runrows AS (
  SELECT session_id, cmd, ndiag, COUNT(*) AS n, MIN(ts) AS t0, MAX(ts) AS t1,
         MIN(replace(substr(IFNULL(diag,''),1,160), char(10), ' ')) AS sample
  FROM (
    SELECT *, SUM(newgrp) OVER (PARTITION BY session_id, cmd ORDER BY id) AS grp
    FROM (
      SELECT *, CASE WHEN ndiag = LAG(ndiag) OVER (PARTITION BY session_id, cmd ORDER BY id)
                     THEN 0 ELSE 1 END AS newgrp
      FROM nd
    )
  ) GROUP BY session_id, cmd, grp
),
hot AS (SELECT * FROM runrows WHERE n >= ${WARN_THRESHOLD} ORDER BY n DESC, t1 DESC LIMIT 10),
p AS (
  SELECT CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.policy') END     AS policy,
         CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.action') END     AS action,
         CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.reasonCode') END AS reason
  FROM win WHERE kind='policy.decision'
),
fault AS (SELECT policy, reason, COUNT(*) AS n FROM p WHERE action='fail-open' GROUP BY 1,2 ORDER BY n DESC),
warns AS (SELECT policy, reason, COUNT(*) AS n FROM p WHERE action='warn' GROUP BY 1,2),
inj AS (
  SELECT CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.path') END   AS path,
         CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.reason') END AS reason,
         CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.bytes') END  AS bytes
  FROM win WHERE kind='constraint.inject'
),
inj_doc AS (SELECT path, COUNT(*) AS n, IFNULL(SUM(CAST(bytes AS INTEGER)),0) AS b FROM inj GROUP BY 1 ORDER BY n DESC),
inj_reason AS (SELECT reason, COUNT(*) AS n FROM inj GROUP BY 1 ORDER BY n DESC),
zero_hit AS (SELECT je.value AS path FROM json_each(${CANDS_LIT}) AS je WHERE je.value NOT IN (SELECT path FROM inj)),
/* ---- ⑦效能看板（harness-effectiveness-metrics） ---- */
/* test-debt-patrol（⑦段 D 子组）：巡检流水只读 events；台账表缺表时用占位 CTE 降级 */
pc AS (SELECT id, ts, CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.ok') END AS ok,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'\$.shard') END AS shard
  FROM win WHERE kind='patrol.check'),
pc_agg AS (SELECT COUNT(*) AS n, MAX(ts) AS last_ts,
    SUM(CASE WHEN ok IN (1,'true') THEN 1 ELSE 0 END) AS ok_n,
    SUM(CASE WHEN ok IN (0,'false') THEN 1 ELSE 0 END) AS fail_n FROM pc),
pc_last AS (SELECT shard FROM pc ORDER BY id DESC LIMIT 1),
${PATROL_DEBT_CTE}
ru_raw AS (SELECT id, session_id, change, payload FROM win WHERE kind='session.rollup' AND json_valid(payload)),
ru AS (SELECT * FROM ru_raw WHERE id IN (SELECT MAX(id) FROM ru_raw GROUP BY session_id)), -- 同 session 取最新一条即终值
ru_t AS (SELECT session_id, COALESCE(change,'') chg,
    CAST(json_extract(payload,'$.turns') AS INTEGER) turns,
    CAST(json_extract(payload,'$.tokens.total') AS INTEGER) tok,
    CAST(json_extract(payload,'$.tokens.output') AS INTEGER) tok_out,
    CAST(json_extract(payload,'$.cost') AS REAL) cost,
    CAST(json_extract(payload,'$.durationSec') AS INTEGER) dur
  FROM ru),
sess_total AS (SELECT COUNT(DISTINCT session_id) n FROM win),
inj_raw AS (SELECT COALESCE(change,'') chg,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.path') END AS path,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.reason') END AS reason
  FROM win WHERE kind='constraint.inject'), -- ⑦段专用：比 inj 多投影 change
inj_sess AS (SELECT session_id, IFNULL(SUM(CAST(CASE WHEN json_valid(payload) THEN json_extract(payload,'$.bytes') END AS INTEGER)),0) b
  FROM win WHERE kind='constraint.inject' GROUP BY 1), -- 混合通道下同 path 不重复记，直接求和（win 直取：inj CTE 无 session_id 列）
inj_deg AS (SELECT COUNT(*) n FROM win WHERE kind='constraint.inject' AND json_valid(payload) AND json_extract(payload,'$.degraded')=1),
pw AS (SELECT COUNT(*) n FROM win WHERE kind='pin.write'),
prd AS (SELECT COUNT(*) n FROM win WHERE kind='pin.read'),
gseq AS (SELECT session_id, cmd, ok, ts,
    LEAD(ok) OVER (PARTITION BY session_id, cmd ORDER BY id) AS nok,
    LEAD(ts)  OVER (PARTITION BY session_id, cmd ORDER BY id) AS nts
  FROM gc),
/* 催修时距分派（add-change-lifecycle-retro design D3）：quality-gate 口径=既有 gate.check
   红→绿（不变）；非 gate 类裁决（ui-design-gate/quota-gate/spec-gate）的解除信号按
   policy 语义分派，信号缺席样本剔除并披露剔除数——统一用 gate.check 转绿判解除对
   非 gate 裁决是错配（实测产出 89 分钟假慢） */
repair_gate AS (SELECT (julianday(nts)-julianday(ts))*86400.0 AS sec FROM gseq WHERE ok=0 AND nok=1),
pb AS (SELECT id, ts, session_id, change, json_extract(payload,'$.policy') policy
  FROM win WHERE kind='policy.decision' AND json_valid(payload)
  AND json_extract(payload,'$.action') IN ('block','warn')
  AND json_extract(payload,'$.policy') IN ('ui-design-gate','quota-gate','spec-gate')),
pass_e AS (SELECT id, ts, change FROM win WHERE kind='policy.decision' AND json_valid(payload)
  AND json_extract(payload,'$.policy')='ui-design-gate' AND json_extract(payload,'$.action')='pass'),
disp_e AS (SELECT id, ts, session_id FROM win WHERE kind='subagent.dispatch' AND json_valid(payload)
  AND NOT COALESCE(json_extract(payload,'$.isError'),0)),
arch_sig AS (SELECT id, ts, COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) AS name
  FROM win WHERE kind='change.archive' AND json_valid(payload)),
pr AS (SELECT b.policy,
    CASE b.policy
      WHEN 'ui-design-gate' THEN (SELECT MIN((julianday(p.ts)-julianday(b.ts))*86400.0) FROM pass_e p WHERE p.id > b.id AND b.change IS NOT NULL AND p.change = b.change)
      WHEN 'quota-gate' THEN (SELECT MIN((julianday(d.ts)-julianday(b.ts))*86400.0) FROM disp_e d WHERE d.id > b.id AND d.session_id = b.session_id)
      WHEN 'spec-gate' THEN (SELECT MIN((julianday(a.ts)-julianday(b.ts))*86400.0) FROM arch_sig a WHERE a.id > b.id AND b.change IS NOT NULL AND a.name = b.change)
    END sec
  FROM pb b),
repair_p AS (SELECT policy, sec FROM pr WHERE sec IS NOT NULL AND sec >= 0),
repair_x AS (SELECT COUNT(*) n FROM pr WHERE sec IS NULL OR sec < 0),
rep_pol AS (SELECT policy, SUM(n) n, SUM(x) x FROM (
    SELECT 'quality-gate' AS policy, COUNT(*) AS n, 0 AS x FROM repair_gate WHERE sec IS NOT NULL AND sec >= 0
    UNION ALL
    SELECT policy, COUNT(*) AS n, 0 AS x FROM repair_p GROUP BY 1
    UNION ALL
    SELECT policy, 0 AS n, COUNT(*) AS x FROM pr WHERE sec IS NULL OR sec < 0 GROUP BY 1
  ) GROUP BY 1),
repair_all AS (SELECT sec FROM repair_gate WHERE sec IS NOT NULL AND sec >= 0 UNION ALL SELECT sec FROM repair_p),
pblock AS (SELECT COALESCE(change,'(无归属)') chg,
    CASE WHEN json_valid(payload) THEN json_extract(payload,'$.reasonCode') END AS reason, COUNT(*) n
  FROM win WHERE kind='policy.decision' AND json_valid(payload) AND json_extract(payload,'$.action')='block' GROUP BY 1,2), -- win 直取：p CTE 无 change 列
dom_map AS (SELECT json_extract(je.value,'$.domain') domain, json_extract(je.value,'$.prefix') prefix FROM json_each(${DOM_LIT}) je),
inj_dom AS (SELECT DISTINCT chg,
    substr(path, instr(path,'flow/')+5, length(path)-instr(path,'flow/')-4-3) domain
  FROM inj_raw WHERE reason IN ('declaration','keyword','jit-path') AND path LIKE '%/flow/%.md'),
inj_unk AS (SELECT COUNT(*) n FROM inj_raw -- 未识别通道显式计数（口径漂移兜底，change: harness-retro-sql-fixes）：
  WHERE path LIKE '%/flow/%.md' AND reason NOT IN ('declaration','keyword','jit-path')), -- 差集限定 flow 文档注入范围，不按全库算（mode-base/change-file/index 不注入 flow 文档，不得误标）
emap_flat AS (SELECT session_id, COALESCE(change,'') chg, je.value p
  FROM win, json_each(json_extract(payload,'$.paths')) je
  WHERE kind='edit.map' AND json_valid(payload)),
emap_dom AS (SELECT DISTINCT e.chg, m.domain FROM emap_flat e JOIN dom_map m ON e.p LIKE (m.prefix || '%') OR e.p LIKE (m.prefix || '/%')),
hit_chg AS (SELECT ed.chg, COUNT(DISTINCT ed.domain) edited,
    COUNT(DISTINCT CASE WHEN idm.domain IS NOT NULL THEN ed.domain END) hit
  FROM emap_dom ed LEFT JOIN inj_dom idm ON idm.chg=ed.chg AND idm.domain=ed.domain GROUP BY 1),
hit_sum AS (SELECT IFNULL(SUM(hit),0) hit, IFNULL(SUM(edited),0) edited FROM hit_chg),
wave AS (SELECT p, COUNT(DISTINCT session_id) n FROM emap_flat GROUP BY 1 HAVING n >= 3 ORDER BY n DESC, p LIMIT 10),
arch_retry AS (SELECT chg, IFNULL(SUM(n),0) n FROM pblock WHERE reason='archive-check-failed' GROUP BY 1 ORDER BY n DESC LIMIT 8),
sub_pairs AS (SELECT id, kind, COALESCE(NULLIF(json_extract(payload,'$.agentId'),NULL),'row'||id) aid,
    CAST(json_extract(payload,'$.tokens') AS INTEGER) t
  FROM win WHERE kind IN ('subagent.dispatch','subagent.complete') AND json_valid(payload)),
sub_one AS (SELECT aid, (SELECT x.t FROM sub_pairs x WHERE x.aid=s.aid AND x.t IS NOT NULL
      ORDER BY CASE WHEN x.kind='subagent.complete' THEN 1 ELSE 0 END DESC, x.id DESC LIMIT 1) t
  FROM (SELECT DISTINCT aid FROM sub_pairs) s),
sub_sum AS (SELECT IFNULL(SUM(t),0) s, COUNT(t) n FROM sub_one WHERE t IS NOT NULL),
ru_sum AS (SELECT IFNULL(SUM(tok),0) s FROM ru_t),
ru_chg AS (SELECT chg, SUM(tok) tok, SUM(cost) cost, SUM(turns) turns FROM ru_t WHERE chg != '' GROUP BY 1),
top_chg AS (SELECT chg, tok, cost FROM ru_chg ORDER BY tok DESC LIMIT 5),
/* 分位（nearest-rank：p50 行号=(n+1)/2、p75 行号=(3n+3)/4，整数除法；空集→NULL） */
q_sturns AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT turns v FROM ru_t WHERE turns IS NOT NULL)),
q_stok AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT tok v FROM ru_t WHERE tok IS NOT NULL)),
q_scost AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT cost v FROM ru_t WHERE cost IS NOT NULL)),
q_sdur AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT dur v FROM ru_t WHERE dur IS NOT NULL)),
q_ctok AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT tok v FROM ru_chg WHERE tok IS NOT NULL)),
q_ccost AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT cost v FROM ru_chg WHERE cost IS NOT NULL)),
q_ib AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT b v FROM inj_sess)),
q_rep AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT sec v FROM repair_all)),
/* ---- B' 每任务成本（task-cost-metrics）：终值三路归因 + 双锚点归档 + 模型/complexity 分桶 ---- */
arch_ev AS (SELECT DISTINCT COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) AS name -- 锚点一：change.archive 事件
  FROM win WHERE kind='change.archive' AND json_valid(payload)
  AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) IS NOT NULL),
arch_dir AS (SELECT DISTINCT json_extract(je.value,'$.name') AS name FROM json_each(${ARCH_LIT}) je), -- 锚点二：archive 目录日期前缀剥名
cx_map AS (SELECT json_extract(je.value,'$.name') name, json_extract(je.value,'$.cx') cx FROM json_each(${ARCH_LIT}) je),
arch_all AS (SELECT name FROM arch_ev UNION SELECT name FROM arch_dir), -- 双锚点并集（单侧缺失以另一侧补齐）
ru_px AS (SELECT ru.session_id, COALESCE(ru.change,'') chg,
    CAST(json_extract(payload,'$.cost') AS REAL) cost,
    json_extract(payload,'$.parentSessionId') parent,
    CASE WHEN json_valid(json_extract(payload,'$.models')) THEN json_extract(payload,'$.models') END models_json
  FROM ru),
ru_attr AS ( -- 三路归因：change 列直归 / parentSessionId 归父（不递归孙会话）/ 无路径 → NULL（不可归因）
  SELECT p.session_id, p.cost, p.models_json,
    CASE WHEN p.chg != '' THEN p.chg
         WHEN p.parent IS NOT NULL THEN (SELECT f.chg FROM ru_px f WHERE f.session_id=p.parent AND f.chg!='')
    END AS attr
  FROM ru_px p),
tc AS (SELECT attr AS name, SUM(cost) AS cost FROM ru_attr -- 每任务成本 = 已归档 change 的归因终值 Σcost（主+子）
  WHERE attr IS NOT NULL AND attr IN (SELECT name FROM arch_all) GROUP BY 1),
q_tc AS (SELECT v, ROW_NUMBER() OVER (ORDER BY v) rn, COUNT(*) OVER () n FROM (SELECT cost v FROM tc)),
tm_flat AS ( -- 模型分桶：归档 change 归因终值的 models map 展开（无 models 字段 → 空，另计未分模型桶）
  SELECT a.attr name, je.key AS model, CAST(json_extract(je.value,'$.cost') AS REAL) mcost
  FROM ru_attr a JOIN ru ON ru.session_id=a.session_id, json_each(a.models_json) je
  WHERE a.attr IN (SELECT name FROM arch_all)),
tm_model AS (SELECT model, SUM(mcost) cost FROM tm_flat GROUP BY 1),
tm_nom AS (SELECT SUM(a.cost) cost FROM ru_attr a -- 缺 models 字段的存量终值整体计入「未分模型」
  WHERE a.attr IN (SELECT name FROM arch_all) AND a.models_json IS NULL),
tcx AS (SELECT COALESCE(cx,'(未声明)') cx, COUNT(*) n, SUM(cost) cost -- complexity 桶：归档目录 proposal 头，无声明计未声明
  FROM tc LEFT JOIN cx_map ON tc.name=cx_map.name GROUP BY 1),
unatt AS (SELECT IFNULL(SUM(cost),0) c FROM ru_attr WHERE attr IS NULL),
tc_total AS (SELECT IFNULL(SUM(cost),0) c FROM ru_attr),
abandon_ev AS (SELECT DISTINCT COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) AS name -- 已放弃终态（add-change-lifecycle-retro：与 change.archive 家族对称）
  FROM win WHERE kind='change.abandon' AND json_valid(payload)
  AND COALESCE(NULLIF(change,''), json_extract(payload,'$.name')) IS NOT NULL),
act_un AS (SELECT COUNT(DISTINCT attr) n FROM ru_attr WHERE attr IS NOT NULL AND attr NOT IN (SELECT name FROM arch_all)
  AND attr NOT IN (SELECT name FROM abandon_ev)), -- 活跃未归档（幸存者偏差对照；已放弃的不占活跃口径）
act_ab AS (SELECT COUNT(DISTINCT attr) n FROM ru_attr WHERE attr IS NOT NULL
  AND attr IN (SELECT name FROM abandon_ev) AND attr NOT IN (SELECT name FROM arch_all)), -- 已放弃（有成本归因无归档锚点）
ab_conf AS (SELECT COUNT(*) n FROM abandon_ev WHERE name IN (SELECT name FROM arch_all)), -- abandon×archive 并存（以归档为准，疑误登记）
arch_evo AS (SELECT COUNT(*) n FROM arch_ev WHERE name NOT IN (SELECT name FROM arch_dir)), -- 仅事件侧（目录被移/重命名）
arch_diro AS (SELECT COUNT(*) n FROM arch_dir WHERE name NOT IN (SELECT name FROM arch_ev)) -- 仅目录侧（事件缺记/TTL 清扫）
SELECT json_object(
  'window', json_object(
    'days', ${DAYS},
    'since', strftime('%Y-%m-%dT%H:%M:%fZ','now','-${DAYS} day'),
    'until', strftime('%Y-%m-%dT%H:%M:%fZ','now'),
    'rows_in_window', (SELECT COUNT(*) FROM win),
    'gate_rows', (SELECT COUNT(*) FROM g),
    'gate_usable_rows', (SELECT COUNT(*) FROM gc),
    'skipped_rows', (SELECT COUNT(*) FROM g WHERE valid=0 OR ok IS NULL),
    'bad_n_rows', (SELECT COUNT(*) FROM gc WHERE ok=1 AND sampled=1 AND n IS NOT NULL AND NOT (typeof(n)='integer' AND n > 0)),
    'oldest_ts', (SELECT MIN(ts) FROM win)
  ),
  'gate', json_object(
    'failures', (SELECT fails FROM agg),
    'runs_restored', (SELECT ok_runs + fails FROM agg),
    'flip_anchors', (SELECT COUNT(*) FROM gc WHERE ok=1 AND flip=1),
    'families', (SELECT json_group_array(json_object('family', fam, 'n', n)) FROM (SELECT fam, COUNT(*) AS n FROM famrows GROUP BY 1 ORDER BY n DESC)),
    'top_domains', (SELECT json_group_array(json_object('domain', dom, 'n', n)) FROM (SELECT dom, COUNT(*) AS n FROM famrows2 GROUP BY 1 ORDER BY n DESC LIMIT 8)),
    'top_cmds', (SELECT json_group_array(json_object('cmd', cmd, 'n', n)) FROM (SELECT cmd, COUNT(*) AS n FROM gc WHERE ok=0 GROUP BY 1 ORDER BY n DESC LIMIT 8))
  ),
  'regression', json_object(
    'regressions', (SELECT COUNT(*) FROM reg),
    'total_failures', (SELECT fails FROM agg),
    'flip_anchors', (SELECT COUNT(*) FROM gc WHERE ok=1 AND flip=1)
  ),
  'faults', json_object(
    'fail_open', (SELECT COUNT(*) FROM p WHERE action='fail-open'),
    'blocks', (SELECT COUNT(*) FROM p WHERE action='block'),
    'by_reason', (SELECT json_group_array(json_object('policy', policy, 'reason', reason, 'n', n)) FROM fault)
  ),
  'soft', json_object(
    'threshold', ${WARN_THRESHOLD},
    'groups_total', (SELECT COUNT(*) FROM warns),
    'warn_total', (SELECT IFNULL(SUM(n),0) FROM warns),
    'warn_total_corrective', (SELECT IFNULL(SUM(n),0) FROM warns WHERE IFNULL(reason,'') NOT IN (${INFORMATIVE_SQL_LIST})),
    'warn_total_informative', (SELECT IFNULL(SUM(n),0) FROM warns WHERE IFNULL(reason,'') IN (${INFORMATIVE_SQL_LIST})),
    'informative', (SELECT json_group_array(json_object('policy', policy, 'reason', reason, 'n', n)) FROM (SELECT * FROM warns WHERE IFNULL(reason,'') IN (${INFORMATIVE_SQL_LIST}) ORDER BY n DESC)),
    'items', (SELECT json_group_array(json_object('policy', policy, 'reason', reason, 'n', n)) FROM (SELECT * FROM warns WHERE n >= ${WARN_THRESHOLD} AND IFNULL(reason,'') NOT IN (${INFORMATIVE_SQL_LIST}) ORDER BY n DESC))
  ),
  'hotspot', json_object(
    'threshold', ${WARN_THRESHOLD},
    'normalized', 1,
    'items', (SELECT json_group_array(json_object('session', session_id, 'cmd', cmd, 'diag', ndiag, 'sample', sample, 'n', n, 'from', t0, 'to', t1)) FROM hot)
  ),
  'inject', json_object(
    'hits', (SELECT COUNT(*) FROM inj),
    'bytes', (SELECT IFNULL(SUM(CAST(bytes AS INTEGER)),0) FROM inj),
    'docs', (SELECT json_group_array(json_object('path', path, 'n', n, 'bytes', b)) FROM (SELECT * FROM inj_doc LIMIT 8)),
    'reasons', (SELECT json_group_array(json_object('reason', reason, 'n', n)) FROM inj_reason),
    'candidates', (SELECT COUNT(*) FROM json_each(${CANDS_LIT})),
    'zero_hit', (SELECT json_group_array(path) FROM zero_hit)
  ),
  'effectiveness', json_object(
    'coverage', json_object(
      'sessions_total', (SELECT n FROM sess_total),
      'rollup_sessions', (SELECT COUNT(*) FROM ru),
      'rollup_events', (SELECT COUNT(*) FROM ru_raw)
    ),
    'inject_load', json_object(
      'sessions_with_inject', (SELECT COUNT(*) FROM inj_sess),
      'per_session_bytes_p50', (SELECT v FROM q_ib WHERE rn = (n+1)/2),
      'per_session_bytes_p75', (SELECT v FROM q_ib WHERE rn = (3*n+3)/4),
      'degraded', (SELECT n FROM inj_deg)
    ),
    'domain_hit', json_object(
      'mapping_rules', (SELECT COUNT(*) FROM dom_map),
      'hit', (SELECT hit FROM hit_sum),
      'edited', (SELECT edited FROM hit_sum),
      'changes_counted', (SELECT COUNT(*) FROM hit_chg),
      'unrecognized_reason_rows', (SELECT n FROM inj_unk)
    ),
    'pin', json_object('write', (SELECT n FROM pw), 'read', (SELECT n FROM prd)),
    'repair', json_object(
      'median_sec', (SELECT v FROM q_rep WHERE rn = (n+1)/2),
      'samples', (SELECT n FROM q_rep),
      'excluded', (SELECT IFNULL(n,0) FROM repair_x),
      'by_policy', (SELECT json_group_array(json_object('policy', policy, 'n', n, 'excluded', x)) FROM (SELECT policy, n, x FROM rep_pol ORDER BY policy))
    ),
    'block_recur', json_object(
      'items', (SELECT json_group_array(json_object('change', chg, 'reason', reason, 'n', n)) FROM (SELECT chg, reason, n FROM pblock WHERE n > 1 ORDER BY n DESC LIMIT 8))
    ),
    'sess_dist', json_object(
      'n', (SELECT COUNT(*) FROM ru_t),
      'turns_p50', (SELECT v FROM q_sturns WHERE rn = (n+1)/2),
      'turns_p75', (SELECT v FROM q_sturns WHERE rn = (3*n+3)/4),
      'tok_p50', (SELECT v FROM q_stok WHERE rn = (n+1)/2),
      'tok_p75', (SELECT v FROM q_stok WHERE rn = (3*n+3)/4),
      'cost_p50', (SELECT v FROM q_scost WHERE rn = (n+1)/2),
      'dur_p50', (SELECT v FROM q_sdur WHERE rn = (n+1)/2)
    ),
    'chg_dist', json_object(
      'n', (SELECT COUNT(*) FROM ru_chg),
      'tok_p50', (SELECT v FROM q_ctok WHERE rn = (n+1)/2),
      'tok_p75', (SELECT v FROM q_ctok WHERE rn = (3*n+3)/4),
      'cost_p50', (SELECT v FROM q_ccost WHERE rn = (n+1)/2),
      'turns_p50', (SELECT v FROM (SELECT turns v, ROW_NUMBER() OVER (ORDER BY turns) rn, COUNT(*) OVER () n FROM ru_chg WHERE turns IS NOT NULL) WHERE rn = (n+1)/2)
    ),
    'top_changes', (SELECT json_group_array(json_object('change', chg, 'tokens', tok, 'cost', cost)) FROM top_chg),
    'subagent', json_object(
      'tokens', (SELECT s FROM sub_sum),
      'samples', (SELECT n FROM sub_sum),
      'rollup_tokens', (SELECT s FROM ru_sum)
    ),
    'waves', json_object(
      'items', (SELECT json_group_array(json_object('path', p, 'sessions', n)) FROM wave)
    ),
    'archive_retry', json_object(
      'items', (SELECT json_group_array(json_object('change', chg, 'blocks', n)) FROM arch_retry)
    ),
    'task_cost', json_object(
      'archived_events', (SELECT COUNT(*) FROM arch_ev),
      'archived_dir', (SELECT COUNT(*) FROM arch_dir),
      'archived_union', (SELECT COUNT(*) FROM arch_all),
      'anchor_event_only', (SELECT n FROM arch_evo),
      'anchor_dir_only', (SELECT n FROM arch_diro),
      'count', (SELECT COUNT(*) FROM tc),
      'cost_p50', (SELECT v FROM q_tc WHERE rn = (n+1)/2),
      'cost_p75', (SELECT v FROM q_tc WHERE rn = (3*n+3)/4),
      'cost_avg', (SELECT CASE WHEN COUNT(*)>0 THEN SUM(cost)/COUNT(*) END FROM tc),
      'top_tasks', (SELECT json_group_array(json_object('change', name, 'cost', cost)) FROM (SELECT name, cost FROM tc ORDER BY cost DESC LIMIT 5)),
      'by_model', (SELECT json_group_array(json_object('model', model, 'cost', cost)) FROM (SELECT model, cost FROM tm_model ORDER BY cost DESC)),
      'unmodel_cost', (SELECT cost FROM tm_nom),
      'by_complexity', (SELECT json_group_array(json_object('complexity', cx, 'n', n, 'cost', cost)) FROM (SELECT cx, n, cost FROM tcx ORDER BY cost DESC)),
      'unattributed_cost', (SELECT c FROM unatt),
      'total_cost', (SELECT c FROM tc_total),
      'active_unarchived', (SELECT n FROM act_un),
      'abandoned', (SELECT n FROM act_ab),
      'abandon_conflicts', (SELECT n FROM ab_conf)
    ),
    'patrol', json_object(
      'checks', (SELECT n FROM pc_agg),
      'ok', (SELECT IFNULL(ok_n,0) FROM pc_agg),
      'failed', (SELECT IFNULL(fail_n,0) FROM pc_agg),
      'last_ts', (SELECT last_ts FROM pc_agg),
      'last_shard', (SELECT shard FROM pc_last),
      'debt_tables', ${PATROL_DEBT_OK},
      'debt', json_object(
        'open', IFNULL((SELECT n FROM debt_status WHERE status='open'),0),
        'fixed', IFNULL((SELECT n FROM debt_status WHERE status='fixed'),0),
        'waived', IFNULL((SELECT n FROM debt_status WHERE status='waived'),0),
        'new_in_window', (SELECT n FROM debt_new),
        'fixed_in_window', (SELECT n FROM debt_fix)
      )
    )
  )
);
SQL
)"

if ! STATS="$(printf '%s\n' "$STATS_SQL" | run_sql 2>&1)"; then
	echo "✗ 统计查询失败: $DB"
	printf '%s\n' "$STATS" | sed 's/^/  /'
	exit 1
fi
case "$STATS" in
'{'*) ;;
*)
	echo "✗ 账本查询未返回预期结果: $DB"
	printf '%s\n' "$STATS" | sed 's/^/  /'
	exit 1
	;;
esac

# ---------- 渲染 / 基线（python3 只做序列化与呈现，不做统计） ----------

TMP_STATS="$(mktemp)"
trap 'rm -f "$TMP_STATS"' EXIT
printf '%s' "$STATS" >"$TMP_STATS"

GEN_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

python3 - "$TMP_STATS" "$JSON_MODE" "$DB" "$CHANGE" "$DAYS" "$WARN_THRESHOLD" \
	"$GEN_AT" "$OPEN_MODE" "$WARNINGS" "$BASELINE" "$SAVE_BASELINE" "$SCRIPT_VERSION" \
	"$DEFAULT_SAMPLE_N" <<'PY'
import json, sys
import unicodedata


def wide(s):
    """终端显示宽度（CJK 字符按 2 列计，避免中文列对齐错位）。"""
    return sum(2 if unicodedata.east_asian_width(ch) in ('W', 'F') else 1 for ch in (s or ''))


def pad(s, width):
    s = s or ''
    return s + ' ' * max(0, width - wide(s))

(stats_path, json_mode, db, change, days, thr, gen_at, open_mode,
 warnings_raw, base_path, save_path, ver, def_n) = sys.argv[1:14]
def_n = int(def_n)

with open(stats_path, encoding='utf-8') as fh:
    st = json.load(fh)

warnings = [w for w in (warnings_raw or '').split('\n') if w.strip()]
win = st['window']

# 降级告警（白盒 T5/T6：口径异常必须显式告知，不得静默）
if win['skipped_rows']:
    warnings.append('窗口内有 %d 条 gate.check 事件的 payload 不可解析或缺判定字段，已跳过（不计入任何分段）'
                    % win['skipped_rows'])
if win['bad_n_rows']:
    warnings.append('窗口内有 %d 条采样事件的 n 非法（非正整数字），已保守按 1 计入执行次数（不放大分母）'
                    % win['bad_n_rows'])
gate = st['gate']
reg = st['regression']
faults = st['faults']
soft = st['soft']
hot = st['hotspot']
inj = st['inject']
eff = st.get('effectiveness') or {}

empty = win['rows_in_window'] == 0


def pct(num, den):
    """整数百分比（半进位），分母为 0 时返回 None（不输出伪 0%）。"""
    if not den:
        return None
    return int((num * 200 + den) // (2 * den))


gate_rate = pct(gate['failures'], gate['runs_restored'])
reg_rate = pct(reg['regressions'], reg['total_failures']) if reg['total_failures'] else None

metrics = {
    'gate.failures': gate['failures'],
    'gate.runs_restored': gate['runs_restored'],
    'gate.fail_rate_pct': gate_rate,
    'gate.regressions': reg['regressions'],
    'gate.flip_anchors': gate['flip_anchors'],
    'harness.fail_open': faults['fail_open'],
    'harness.blocks': faults['blocks'],
    # stale_groups 语义=纠正型失效组数（soft.items 已按情报型白名单剔除；键名不变保基线连续）
    'soft.stale_groups': len(soft['items']),
    'soft.warn_total': soft['warn_total'],
    'soft.warn_total_corrective': soft['warn_total_corrective'],
    'soft.warn_total_informative': soft['warn_total_informative'],
    'soft.informative_groups': len(soft.get('informative') or []),
    'hotspot.runs': len(hot['items']),
    'inject.hits': inj['hits'],
    'inject.bytes': inj['bytes'],
    'inject.zero_hit_docs': len(inj['zero_hit']),
    'window.skipped_rows': win['skipped_rows'],
    'window.bad_n_rows': win['bad_n_rows'],
}

# ⑦效能看板扁平键（m7.*；缺 effect 段时全部置 None，旧库/异常不伪造）
def _e(path, default=None):
    node = eff
    for k in path.split('.'):
        if not isinstance(node, dict):
            return default
        node = node.get(k, default)
    return node


def _pct(num, den):
    if not den:
        return None
    return int((num * 200 + den) // (2 * den))


_cov = _e('coverage') or {}
_dom = _e('domain_hit') or {}
metrics.update({
    'm7.rollup_coverage_pct': _pct(_cov.get('rollup_sessions', 0), _cov.get('sessions_total', 0)),
    'm7.rollup_sessions': _cov.get('rollup_sessions'),
    'm7.inj_bytes_p50': _e('inject_load.per_session_bytes_p50'),
    'm7.inj_bytes_p75': _e('inject_load.per_session_bytes_p75'),
    'm7.inj_degraded': _e('inject_load.degraded'),
    'm7.domain_hit_pct': _pct(_dom.get('hit', 0), _dom.get('edited', 0)),
    'm7.pin_write': _e('pin.write'),
    'm7.pin_read': _e('pin.read'),
    'm7.repair_med_sec': _e('repair.median_sec'),
    'm7.repair_samples': _e('repair.samples'),
    'm7.repair_excluded': _e('repair.excluded'),
    'm7.block_recur_groups': len(_e('block_recur.items') or []),
    'm7.sess_tok_p50': _e('sess_dist.tok_p50'),
    'm7.sess_tok_p75': _e('sess_dist.tok_p75'),
    'm7.sess_cost_p50': _e('sess_dist.cost_p50'),
    'm7.sess_dur_p50': _e('sess_dist.dur_p50'),
    'm7.chg_tok_p50': _e('chg_dist.tok_p50'),
    'm7.chg_tok_p75': _e('chg_dist.tok_p75'),
    'm7.sub_samples': _e('subagent.samples'),
    'm7.wave_files': len(_e('waves.items') or []),
    'm7.archive_retry_changes': len(_e('archive_retry.items') or []),
})

# B' 每任务成本（task-cost-metrics）：分布 + 分桶 + 不可归因披露；旧库无 task_cost 段时全 None
_tc = _e('task_cost') or {}
_by_model = {it['model']: it.get('cost') for it in (_tc.get('by_model') or [])}
if _tc.get('unmodel_cost') is not None:
    _by_model['(未分模型)'] = _tc.get('unmodel_cost')
_by_cx = {it['complexity']: it.get('cost') for it in (_tc.get('by_complexity') or [])}
metrics.update({
    'm7.task_cost_p50': _tc.get('cost_p50'),
    'm7.task_cost_p75': _tc.get('cost_p75'),
    'm7.task_cost_avg': _tc.get('cost_avg'),
    'm7.task_cost_count': _tc.get('count'),
    'm7.task_cost_by_model': _by_model if _tc else None,
    'm7.task_cost_by_complexity': _by_cx if _tc else None,
    'm7.task_unattributed_cost_pct': _pct(_tc.get('unattributed_cost') or 0, _tc.get('total_cost') or 0) if _tc else None,
    'm7.task_active_unarchived': _tc.get('active_unarchived'),
    'm7.task_abandoned': _tc.get('abandoned'),
    'm7.task_abandon_conflict': _tc.get('abandon_conflicts'),
})

# ⑧/D 组：测试欠账巡检（test-debt-patrol；patrol.check 流水 + test_debt 台账，缺表/零事件时 None）
_patrol = _e('patrol') or {}
if _patrol:
    _debt = _patrol.get('debt') or {}
    metrics.update({
        'patrol.checks': _patrol.get('checks'),
        'patrol.ok': _patrol.get('ok'),
        'patrol.failed': _patrol.get('failed'),
        'patrol.debt_open': _debt.get('open') if _patrol.get('debt_tables') else None,
        'patrol.debt_fixed': _debt.get('fixed') if _patrol.get('debt_tables') else None,
        'patrol.debt_waived': _debt.get('waived') if _patrol.get('debt_tables') else None,
        'patrol.debt_new_in_window': _debt.get('new_in_window') if _patrol.get('debt_tables') else None,
        'patrol.debt_fixed_in_window': _debt.get('fixed_in_window') if _patrol.get('debt_tables') else None,
    })

doc = {
    'meta': {
        'script_version': ver,
        'db': db,
        'open_mode': open_mode,
        'change': change or None,
        'days': int(days),
        'warn_threshold': int(thr),
        'generated_at': gen_at,
        'since': win['since'],
        'until': win['until'],
        'empty': empty,
    },
    'metrics': metrics,
    'window': win,
    'gate': gate,
    'regression': reg,
    'faults': faults,
    'soft': soft,
    'hotspot': hot,
    'inject': inj,
    'effectiveness': eff or None,
    'warnings': warnings,
}

# ---- 基线读写 ----
baseline_diff = None
if save_path:
    snap = {
        'version': int(ver),
        'generated_at': gen_at,
        'days': int(days),
        'change': change or None,
        'window': {'since': win['since'], 'until': win['until']},
        'metrics': metrics,
    }
    try:
        with open(save_path, 'w', encoding='utf-8') as fh:
            json.dump(snap, fh, ensure_ascii=False, indent=2, sort_keys=True)
            fh.write('\n')
        warnings.append('基线已写入 %s' % save_path)
    except OSError as exc:
        warnings.append('基线写入失败（%s）：%s' % (save_path, exc))

if base_path:
    try:
        with open(base_path, encoding='utf-8') as fh:
            base = json.load(fh)
        base_metrics = base['metrics']
        if not isinstance(base_metrics, dict):
            raise ValueError('metrics 非对象')
        if not any(k.startswith('m7.') for k in base_metrics):
            warnings.append('基线为旧版（无 m7.* 效能键）：效能看板段差值不可比，已跳过')
        def delta(k):
            a, b = metrics.get(k), base_metrics.get(k)
            if not isinstance(a, (int, float)) or not isinstance(b, (int, float)):
                return None
            return a - b

        baseline_diff = {
            'path': base_path,
            'generated_at': base.get('generated_at'),
            'days': base.get('days'),
            'change': base.get('change'),
            'diff': {k: delta(k) for k in sorted(set(metrics) | set(base_metrics))},
        }
    except (OSError, ValueError, KeyError, TypeError) as exc:
        warnings.append('基线不可用、已跳过比对（%s）：%s' % (base_path, exc))

if baseline_diff is not None:
    doc['baseline'] = baseline_diff

if json_mode == '1':
    json.dump(doc, sys.stdout, ensure_ascii=False, indent=2, sort_keys=True)
    sys.stdout.write('\n')
    sys.exit(0)

# ---- 文本渲染 ----
out = []
out.append('harness-retro — harness 事实账本失败聚类报告')
out.append('账本      : %s（%s 打开，只读）' % (db, open_mode))
out.append('窗口      : 最近 %s 天（UTC，左闭）%s ~ %s' % (days, win['since'], win['until']))
out.append('归属      : %s' % ('change = %s' % change if change else '全部 change'))
out.append('软提醒阈值: %s' % thr)
if win['oldest_ts']:
    out.append('最早事件  : %s' % win['oldest_ts'])
out.append('')

if empty:
    out.append('窗口内无数据：最近 %s 天内没有任何事件（不输出任何比率指标）' % days)
else:
    rate_txt = '—' if gate_rate is None else '%d%%' % gate_rate
    out.append('① 门禁失败聚类')
    out.append('   失败条数            : %s' % gate['failures'])
    out.append('   还原执行次数（分母）: %s' % gate['runs_restored'])
    out.append('   失败率              : %s' % rate_txt)
    out.append('   口径                : 失败条计 1；成功锚点计 1；采样条按 n 还原（缺 n 回退 %s；n 非法保守计 1）'
               % def_n)
    out.append('   转绿锚点数          : %s' % gate['flip_anchors'])
    if gate['families']:
        out.append('   按失败特征归并      :')
        for it in gate['families']:
            out.append('     - %s %s' % (pad(it['family'], 26), it['n']))
    if gate['top_domains']:
        out.append('   按 domain 包（Top） :')
        for it in gate['top_domains']:
            out.append('     - %s %s' % (pad((it['domain'] or '(非包命令)')[:28], 30), it['n']))
    if gate['top_cmds']:
        out.append('   Top 失败命令        :')
        for it in gate['top_cmds']:
            out.append('     - %s %s' % (pad((it['cmd'] or '(无 cmd)')[:52], 54), it['n']))
    out.append('')

    out.append('② 回归翻转（同一 session+cmd 曾绿变红）')
    out.append('   回归失败数          : %s' % reg['regressions'])
    out.append('   占窗口内失败比例    : %s' % ('—' if reg_rate is None else '%d%%' % reg_rate))
    out.append('   转绿锚点（修复信号）: %s' % reg['flip_anchors'])
    out.append('')

    out.append('③ harness 自身故障（action=fail-open，单列，不计入 agent 失败）')
    out.append('   故障条数            : %s' % faults['fail_open'])
    for it in faults['by_reason']:
        out.append('     - %s / %s : %s' % (it['policy'], it['reason'], it['n']))
    if not faults['by_reason']:
        out.append('     （无）')
    out.append('   另有 block 裁决     : %s（策略正常工作，不计入故障段）' % faults['blocks'])
    out.append('')

    out.append('④ 软提醒失效（纠正型；同 policy+reasonCode 的 warn 计数 ≥ %s）' % soft['threshold'])
    out.append('   失效组数            : %s / %s（本窗口 warn 共 %s 条）'
               % (len(soft['items']), soft['groups_total'], soft['warn_total']))
    for it in soft['items']:
        out.append('     - %s / %s : %s 次' % (it['policy'], it['reason'], it['n']))
    if not soft['items']:
        out.append('     （无：纠正型软提醒未达到重复阈值）')
    inf = soft.get('informative') or []
    inf_total = soft.get('warn_total_informative') or 0
    if inf:
        out.append('   情报型 warn（归属/状态告知，不计失效）: %s 条（%s）'
                   % (inf_total, ' / '.join('%s/%s %s' % (it['policy'], it['reason'], it['n']) for it in inf)))
    else:
        out.append('   情报型 warn（归属/状态告知，不计失效）: （无）')
    out.append('')

    out.append('⑤ 重复失败热点（同会话同命令、归一化 diag 连续重复 ≥ %s：死循环信号）' % hot['threshold'])
    for it in hot['items']:
        out.append('     - [%s] %s ×%s' % (it['session'], (it['cmd'] or '(无 cmd)')[:48], it['n']))
        out.append('       diag: %s' % (it.get('sample') or it['diag'] or '')[:140])
        out.append('       区间: %s ~ %s' % (it['from'], it['to']))
    if not hot['items']:
        out.append('     （无：没有连续重复的失败序列）')
    out.append('')

    out.append('⑥ 注入面健康（constraint.inject）')
    out.append('   注入条数 / 字节    : %s / %s' % (inj['hits'], inj['bytes']))
    for it in inj['docs']:
        out.append('     - %s %s 次 / %s B' % (pad((it['path'] or '(无 path)')[:50], 52), it['n'], it['bytes']))
    if inj['zero_hit']:
        out.append('   窗口内零命中文档（索引声明但未被注入：死约束候选）: %s' % len(inj['zero_hit']))
        for p in inj['zero_hit'][:8]:
            out.append('     - %s' % p)
    elif inj['candidates']:
        out.append('   窗口内零命中文档: 0（候选 %s 篇均被注入过）' % inj['candidates'])
    else:
        out.append('   窗口内零命中文档: 未知（未找到候选索引）')
    out.append('')

    # ⑦效能看板（harness-effectiveness-metrics：A 插件 ROI / B 效率基线 / C 返工信号）
    out.append('⑦ 效能看板（插件 ROI / 效率基线 / 返工信号；分位=nearest-rank，各指标独立解读）')
    cov_n = _cov.get('rollup_sessions') or 0
    cov_d = _cov.get('sessions_total') or 0
    cov_pct = _pct(cov_n, cov_d)
    accumulating = (cov_pct is None) or cov_pct < 50
    if cov_d:
        out.append('   rollup 覆盖率        : %s / %s session%s'
                   % (cov_n, cov_d,
                      ('（%d%%）⚠ 数据积累中（<50%%，分布仅由已覆盖 session 计算）' % cov_pct) if accumulating else '（%d%%）' % cov_pct))
    else:
        out.append('   rollup 覆盖率        : 窗口无 session')

    out.append('   A 插件 ROI')
    il = _e('inject_load') or {}
    p50 = il.get('per_session_bytes_p50')
    p75 = il.get('per_session_bytes_p75')
    out.append('     注入负载          : per-session 字节 中位 %s / P75 %s（%s session 有注入；degraded %s 条）'
               % ('—' if p50 is None else '%d B' % p50,
                  '—' if p75 is None else '%d B' % p75,
                  il.get('sessions_with_inject') or 0,
                  il.get('degraded') or 0))
    if (_dom.get('mapping_rules') or 0) > 0:
        dh = _pct(_dom.get('hit', 0), _dom.get('edited', 0))
        out.append('     注入命中率        : 编辑域命中注入域 %s / %s（%s；域映射=flow 头 doc-impact-applies，%s 条规则）'
                   % (_dom.get('hit') or 0, _dom.get('edited') or 0,
                      '—' if dh is None else '%d%%' % dh,
                      _dom.get('mapping_rules')))
        unk = _dom.get('unrecognized_reason_rows') or 0
        out.append('     未识别通道        : %s 条（flow 文档注入的 reason 不在 declaration/keyword/jit-path 枚举内%s）'
                   % (unk, '；⚠ 注入通道枚举已漂移，须同步报告口径（勿静默归零）' if unk else ''))
    else:
        out.append('     注入命中率        : 域映射不可用（未解析到 doc-impact-applies 标签，跳过本指标）')
    out.append('     pin 复用          : write %s 条 / read %s 条（比值 %s）'
               % (_e('pin.write') or 0, _e('pin.read') or 0,
                  '—' if not _e('pin.write') else '%.1f' % ((_e('pin.read') or 0) / _e('pin.write'))))
    rep_med = _e('repair.median_sec')
    rep_n = _e('repair.samples') or 0
    rep_ex = _e('repair.excluded') or 0
    out.append('     门禁催修时距      : 中位 %s（n=%s；分派：quality-gate=转绿 / ui-design-gate=pass 放行 / quota-gate=成功派发 / spec-gate=归档%s）'
               % ('—' if rep_med is None else '%.1f min' % (rep_med / 60.0), rep_n,
                  '；剔除无解除信号 %s 条' % rep_ex if rep_ex else ''))
    recur = _e('block_recur.items') or []
    out.append('     block 复发        : %s 组（同 change×reasonCode >1 次）' % len(recur))
    for it in recur[:5]:
        out.append('       - %s / %s : %s 次' % (it['change'], it['reason'], it['n']))

    out.append('   B 效率基线（session.rollup 终值驱动%s）'
               % ('，⚠ 数据积累中' if accumulating else ''))
    sd = _e('sess_dist') or {}
    if (sd.get('n') or 0) > 0:
        out.append('     per-session       : turns 中位 %s | tokens 中位 %s / P75 %s | cost 中位 %s | 时长中位 %s'
                   % (sd.get('turns_p50'), sd.get('tok_p50'), sd.get('tok_p75'),
                      '—' if sd.get('cost_p50') is None else '¥%.2f' % sd['cost_p50'],
                      '—' if sd.get('dur_p50') is None else '%.1f min' % (sd['dur_p50'] / 60.0)))
    else:
        out.append('     per-session       : （无 rollup 终值：分布不产出，不用小样本冒充）')
    cd = _e('chg_dist') or {}
    if (cd.get('n') or 0) > 0:
        out.append('     per-change        : tokens 中位 %s / P75 %s | cost 中位 %s（%s change）'
                   % (cd.get('tok_p50'), cd.get('tok_p75'),
                      '—' if cd.get('cost_p50') is None else '¥%.2f' % cd['cost_p50'], cd.get('n')))
    else:
        out.append('     per-change        : （无归属 change 的 rollup 终值）')
    for it in (_e('top_changes') or [])[:5]:
        out.append('       - %s : tokens %s / cost %s'
                   % (pad((it['change'] or '')[:40], 42),
                      it.get('tokens'), '—' if it.get('cost') is None else '¥%.2f' % it['cost']))
    sub = _e('subagent') or {}
    sub_tok = sub.get('tokens') or 0
    ru_tok = sub.get('rollup_tokens') or 0
    if (sub.get('samples') or 0) > 0 and (sub_tok + ru_tok) > 0:
        out.append('     子线程 token 占比 : %d%%（子线程 %s / 合计 %s；非空样本 %s 条）'
                   % (_pct(sub_tok, sub_tok + ru_tok), sub_tok, sub_tok + ru_tok, sub['samples']))
    else:
        out.append('     子线程 token 占比 : —（无子线程 token 样本）')

    # B' 每任务成本（task-cost-metrics：已归档 change × 归因终值 Σcost；双锚点=change.archive 事件 ∪ archive 目录）
    out.append("   B' 每任务成本（归档 change × 归因会话终值 Σcost；双锚点=change.archive 事件 ∪ archive 目录）")
    _tcd = _e('task_cost') or {}
    if (_tcd.get('count') or 0) > 0:
        out.append('     分布              : P50 %s / P75 %s / 均值 %s（%s 个归档任务）'
                   % ('—' if _tcd.get('cost_p50') is None else '¥%.2f' % _tcd['cost_p50'],
                      '—' if _tcd.get('cost_p75') is None else '¥%.2f' % _tcd['cost_p75'],
                      '—' if _tcd.get('cost_avg') is None else '¥%.2f' % _tcd['cost_avg'],
                      _tcd.get('count')))
    else:
        out.append('     分布              : （窗口内无已归档 change：分布不产出，不用小样本冒充）')
    _bm = _by_model if _by_model else {}
    if _bm:
        _bm_tot = sum(v for v in _bm.values() if isinstance(v, (int, float))) or None
        segs = ['%s %s%s' % (pad((k or '(无模型)')[:30], 32),
                             '—' if v is None else '¥%.2f' % v,
                             '' if (not _bm_tot or not isinstance(v, (int, float))) else '（%d%%）' % _pct(v, _bm_tot))
                for k, v in sorted(_bm.items(), key=lambda kv: -(kv[1] or 0))]
        out.append('     按模型分桶        : %s' % '；'.join(segs))
    else:
        out.append('     按模型分桶        : —（归档任务终值均无 models 字段且无成本）')
    _bcx = _e('task_cost.by_complexity') or []
    if _bcx:
        out.append('     按 complexity 分桶:')
        for it in _bcx:
            out.append('       - %s : %s 个任务 / 合计 %s'
                       % (pad(it['complexity'], 22), it['n'],
                          '—' if it.get('cost') is None else '¥%.2f' % it['cost']))
    _un = _tcd.get('unattributed_cost')
    if _un is not None and _un > 0:
        out.append('     不可归因成本      : %s（占窗口终值 %s%%；change 列空且无父链的会话成本，不计入任务均值）'
                   % ('¥%.2f' % _un, _pct(_un, _tcd.get('total_cost') or 0)))
    else:
        out.append('     不可归因成本      : 0')
    out.append('     活跃未归档 change : %s 个（窗口内有成本归因但未归档且未登记放弃：幸存者偏差对照，成本不在分布内）'
               % (_tcd.get('active_unarchived') or 0))
    _abn = _tcd.get('abandoned') or 0
    if _abn:
        out.append('     已放弃 change     : %s 个（change.abandon 登记：不再活跃也不会归档，成本不在分布内——完整分母要加回）' % _abn)
    _abc = _tcd.get('abandon_conflicts') or 0
    if _abc:
        out.append('     ⚠ 终态冲突        : %s 个 change 名下 abandon×archive 并存，以归档为准（abandon 疑为误登记，见 scripts/harness/change-abandon.sh）' % _abc)
    _evo = _tcd.get('anchor_event_only') or 0
    _diro = _tcd.get('anchor_dir_only') or 0
    if _evo or _diro:
        out.append('     锚点不对称提示    : 仅事件侧 %s 个（目录被移/重命名）、仅目录侧 %s 个（事件缺记或保留期已过，已目录兑底）'
                   % (_evo, _diro))
    for it in (_tcd.get('top_tasks') or [])[:5]:
        out.append('       - %s : %s'
                   % (pad((it['change'] or '')[:40], 42),
                      '—' if it.get('cost') is None else '¥%.2f' % it['cost']))

    out.append('   C 返工信号')
    waves = _e('waves.items') or []
    out.append('     跨 session 编辑波次: %s 个文件 ≥3 session（累计快照口径：含被动携带）' % len(waves))
    for it in waves[:8]:
        out.append('       - %s ×%s session' % (pad((it['path'] or '')[:56], 58), it['sessions']))
    ars = _e('archive_retry.items') or []
    out.append('     归档重试          : %s 个 change 有归档门禁 block（spec-gate 拦截后重试）' % len(ars))
    for it in ars[:5]:
        out.append('       - %s : %s 次' % (pad((it['change'] or '')[:40], 42), it['blocks']))

    # D 测试欠账巡检（test-debt-patrol：patrol.check 流水 + test_debt 台账；缺表/零事件降级不报错）
    pt = _e('patrol') or {}
    out.append('   D 测试欠账巡检（patrol.check 流水 / test_debt 台账）')
    p_n = pt.get('checks') or 0
    if p_n:
        p_ok = pt.get('ok') or 0
        p_rate = _pct(p_ok, p_n)
        out.append('     巡检频次          : %s 次（ok %s / 失败 %s，%s）最近一次 %s%s'
                   % (p_n, p_ok, pt.get('failed') or 0,
                      '—' if p_rate is None else '%d%% ok' % p_rate,
                      pt.get('last_ts') or '—',
                      '' if not pt.get('last_shard') else '（最近分片 %s）' % pt['last_shard']))
    else:
        out.append('     巡检频次          : 无巡检数据（窗口内无 patrol.check 事件）')
    if pt.get('debt_tables'):
        pd = pt.get('debt') or {}
        out.append('     台账欠账          : open %s / fixed %s / waived %s；窗口内新增 %s、修复 %s（口径：first_seen/last_seen 自然日）'
                   % (pd.get('open') or 0, pd.get('fixed') or 0, pd.get('waived') or 0,
                      pd.get('new_in_window') or 0, pd.get('fixed_in_window') or 0))
    else:
        out.append('     台账欠账          : 无巡检数据（test_debt 表不存在或未初始化，跳过）')
    out.append('')

if baseline_diff is not None:
    out.append('较基线变化（基线 %s，生成于 %s）' % (baseline_diff['path'], baseline_diff['generated_at']))
    for k, d in baseline_diff['diff'].items():
        if d is None and isinstance(metrics.get(k), (int, float)):
            out.append('   %-24s 新增（基线无此键）' % k)
        else:
            out.append('   %-24s %s' % (k, '—' if d is None else '%+d' % d))
    out.append('')

if warnings:
    out.append('提示 / 降级:')
    for w in warnings:
        out.append('  ⚠ %s' % w)
    out.append('')

sys.stdout.write('\n'.join(out))
PY
