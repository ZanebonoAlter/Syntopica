#!/usr/bin/env bash
# harness-retro.sh — harness 事实账本的失败聚类复盘报告（change: harness-retro-loop）
#
# 用法：bash scripts/harness-retro.sh [选项]
#   --db PATH              目标账本（缺省 .pi/harness/events.db）
#   --days N               统计窗口天数（缺省 7）
#   --change NAME          只统计 change 列归属该值的事件（缺省全局）
#   --warn-threshold N     软提醒失效阈值（缺省 5）
#   --json                 输出机器可读 JSON（与文本同结构）
#   --save-baseline [PATH] 落基线快照（缺省 .pi/harness/retro-baseline.json）
#   --baseline [PATH]      与基线比对并输出变化量（同上缺省路径）
#   --help                 打印用法
#
# 只读消费：sqlite3 -readonly 打开 + PRAGMA application_id 校验（魔数 0x53594E54），
# 不建表、不写事件、不触碰注入通道配置（报告只以命令输出形态存在）。
# 六段报告：① 门禁失败聚类 ② 回归翻转 ③ harness 自身故障 ④ 软提醒失效 ⑤ 重复失败热点 ⑥ 注入面健康
#
# 分母口径（与 harness-fact-log 的 gate.check 采样记账协议一致）：
#   ok=false 计 1；ok=true 无采样标记（会话首成功/转绿锚点）计 1；
#   ok=true 且 sampled=true 按 payload.n 计（缺 n 回退缺省 N=5；n 非法保守计 1 并告警）。
#
# 退出码：0 报告成功（含窗口内无数据、发现大量失败）；1 参数或环境错误（库缺失/非本应用库/损坏）。
# 依赖：bash、sqlite3（≥3.33，含 json1 与窗口函数）、python3（渲染与基线读写）、awk 不需要。
# 契约锚：openspec/changes/harness-retro-loop/specs/harness-retro-loop/spec.md
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SCRIPT_VERSION="1"
APP_ID_DEC=1398361684 # 0x53594E54 = 'S''Y''N''T'
DEFAULT_SAMPLE_N=5    # 与 .pi/extensions/lib/gate-sample.ts 的 GATE_OK_SAMPLE_EVERY 一致
TTL_DAYS_MIN=30       # 被消费 kind 的最短保留期（session.start 为 90 天）
HOTSPOT_DEFAULT_THR=5

DB=".pi/harness/events.db"
DAYS=7
CHANGE=""
WARN_THRESHOLD=$HOTSPOT_DEFAULT_THR
JSON_MODE=0
SAVE_BASELINE=""
BASELINE=""
BASELINE_DEFAULT=".pi/harness/retro-baseline.json"

usage() {
	cat <<'USAGE'
harness-retro — harness 事实账本的失败聚类复盘报告（只读）

用法: bash scripts/harness-retro.sh [选项]

  --db PATH               目标账本（缺省 .pi/harness/events.db）
  --days N                统计窗口天数（缺省 7；超过 30 天会提示 TTL 清扫风险）
  --change NAME           只统计 change 列归属该值的事件（缺省全局）
  --warn-threshold N      软提醒失效阈值（缺省 5；同 policy+reasonCode 的 warn 计数 ≥ 阈值即列出）
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

WARNINGS=""
add_warning() { WARNINGS="${WARNINGS}${WARNINGS:+
}$1"; }

if [ "$DAYS" -gt "$TTL_DAYS_MIN" ]; then
	add_warning "窗口 ${DAYS} 天 > 被消费事件类型的最短保留期 ${TTL_DAYS_MIN} 天：该窗口可能已被 TTL 清扫，与基线的差值可能失真（数据被清扫不等于指标改善）"
fi
if [ "$OPEN_MODE" = "immutable" ]; then
	add_warning "账本以 immutable=1 回退方式打开（只读打开失败）：可能少最后几条未 checkpoint 的事件"
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
    WHEN diag LIKE '%parallel golangci-lint is running%' OR diag LIKE '%text file busy%'
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
zero_hit AS (SELECT je.value AS path FROM json_each(${CANDS_LIT}) AS je WHERE je.value NOT IN (SELECT path FROM inj))
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
    'items', (SELECT json_group_array(json_object('policy', policy, 'reason', reason, 'n', n)) FROM (SELECT * FROM warns WHERE n >= ${WARN_THRESHOLD} ORDER BY n DESC))
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
    'soft.stale_groups': len(soft['items']),
    'soft.warn_total': soft['warn_total'],
    'hotspot.runs': len(hot['items']),
    'inject.hits': inj['hits'],
    'inject.bytes': inj['bytes'],
    'inject.zero_hit_docs': len(inj['zero_hit']),
    'window.skipped_rows': win['skipped_rows'],
    'window.bad_n_rows': win['bad_n_rows'],
}

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

    out.append('④ 软提醒失效（同 policy+reasonCode 的 warn 计数 ≥ %s）' % soft['threshold'])
    out.append('   失效组数            : %s / %s（本窗口 warn 共 %s 条）'
               % (len(soft['items']), soft['groups_total'], soft['warn_total']))
    for it in soft['items']:
        out.append('     - %s / %s : %s 次' % (it['policy'], it['reason'], it['n']))
    if not soft['items']:
        out.append('     （无：软提醒未达到重复阈值）')
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

if baseline_diff is not None:
    out.append('⑦ 较基线变化（基线 %s，生成于 %s）' % (baseline_diff['path'], baseline_diff['generated_at']))
    for k, d in baseline_diff['diff'].items():
        out.append('   %-24s %s' % (k, '—' if d is None else '%+d' % d))
    out.append('')

if warnings:
    out.append('提示 / 降级:')
    for w in warnings:
        out.append('  ⚠ %s' % w)
    out.append('')

sys.stdout.write('\n'.join(out))
PY
