#!/usr/bin/env bash
# backup-key-tables.smoke.sh — 备份脚本冒烟（change: fix-disaster-recovery-channel）
#
# 覆盖场景：
#   - 排除清单恰为 14 张可再生表（schema_migrations 已移出）
#   - 排除清单内的表不进备份档
#   - 迁移历史（schema_migrations）进入备份档
#   - 新建业务表自动被备份
#   - 行数基线清单产出且与实际行数一致
#   - 轮转后 .counts.tsv 数与 .dump 数一致（基线不孤儿）
#
# 全程在一次性测试库 + 临时目录上跑，绝不碰生产库与 backups/ 真档。
# 退出码：0 通过；1 有失败。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

CONTAINER="${BACKUP_CONTAINER:-syntopica-postgres}"
PG_USER="${BACKUP_PG_USER:-postgres}"
PG_PASSWORD="${BACKUP_PG_PASSWORD:-postgres}"
PG_HOST="${BACKUP_PG_HOST:-127.0.0.1}"
PG_PORT="${BACKUP_PG_PORT:-5432}"
SMOKE_DB="syntopica_smoke_backup_$$"
TMP_DIR="$(mktemp -d)"
SCRIPT="scripts/db/backup-key-tables.sh"

PASS=0
FAIL=0
ok() {
	echo "  [OK]   $1"
	PASS=$((PASS + 1))
}
fail() {
	echo "  [FAIL] $1"
	FAIL=$((FAIL + 1))
}

cleanup() {
	PGPASSWORD="$PG_PASSWORD" dropdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" --if-exists "$SMOKE_DB" >/dev/null 2>&1 || true
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT

psql_smoke() {
	PGPASSWORD="$PG_PASSWORD" psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$SMOKE_DB" -v ON_ERROR_STOP=1 "$@"
}

echo "== A. 排除清单静态断言（恰 14 张，schema_migrations 不在内） =="
EXCLUDED_BLOCK="$(awk '/^EXCLUDED=\(/,/^\)/' "$SCRIPT")"
EXCLUDED_LIST="$(printf '%s\n' "$EXCLUDED_BLOCK" | grep -E '^[[:space:]]+[a-z_][a-z0-9_]*$' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
# 排除清单的语义与顺序均不得变（tasks 1.2 给出的原始顺序）
EXPECTED="ai_call_logs
otel_spans
ai_embedding_cache
firecrawl_jobs
tag_jobs
embedding_queues
merge_reembedding_queues
notifications
discovery_runs
discovery_run_items
cross_board_relation_runs
topic_watch_hits
scheduler_tasks
reading_behaviors"
if [ "$EXCLUDED_LIST" = "$EXPECTED" ]; then
	ok "排除清单恰为 14 张（$(printf '%s\n' "$EXCLUDED_LIST" | grep -c .) 张）"
else
	fail "排除清单与预期不符："
	printf '%s\n' "$EXCLUDED_LIST" | sed 's/^/         实际: /'
fi
if printf '%s\n' "$EXCLUDED_BLOCK" | grep -q "schema_migrations"; then
	fail "EXCLUDED 段内仍含 schema_migrations"
else
	ok "EXCLUDED 段内无 schema_migrations（迁移历史必须随档走）"
fi

echo ""
echo "== B. 一次性测试库准备 =="
PGPASSWORD="$PG_PASSWORD" dropdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" --if-exists "$SMOKE_DB" >/dev/null 2>&1 || true
if ! PGPASSWORD="$PG_PASSWORD" createdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" "$SMOKE_DB" >/dev/null 2>&1; then
	fail "无法创建一次性测试库 $SMOKE_DB"
	echo "冒烟未完成（生产库未被触碰）"
	exit 1
fi
ok "测试库 $SMOKE_DB 已创建"

psql_smoke -q <<'SQL' >/dev/null
CREATE TABLE keep_me (id bigserial PRIMARY KEY, name text);
CREATE TABLE another_business_table (id bigserial PRIMARY KEY);
CREATE TABLE ai_call_logs (id bigserial PRIMARY KEY, payload text);
CREATE TABLE schema_migrations (version text PRIMARY KEY);
INSERT INTO keep_me (name) SELECT 'row-' || g FROM generate_series(1, 7) g;
INSERT INTO another_business_table SELECT g FROM generate_series(1, 3) g;
INSERT INTO ai_call_logs (payload) SELECT 'log' FROM generate_series(1, 5) g;
INSERT INTO schema_migrations (version) VALUES ('20260403_0001'), ('20260403_0003');
SQL
ok "测试表就绪（keep_me / another_business_table / ai_call_logs / schema_migrations）"

echo ""
echo "== C. 跑一次备份（临时目录，KEEP=7） =="
if BACKUP_DB="$SMOKE_DB" BACKUP_DIR="$TMP_DIR" BACKUP_KEEP=7 \
	BACKUP_CONTAINER="$CONTAINER" BACKUP_PG_HOST="$PG_HOST" BACKUP_PG_PORT="$PG_PORT" \
	BACKUP_PG_USER="$PG_USER" BACKUP_PG_PASSWORD="$PG_PASSWORD" \
	bash "$SCRIPT" >"$TMP_DIR/run1.log" 2>&1; then
	ok "备份脚本退出码 0"
else
	fail "备份脚本非零退出："
	sed 's/^/         /' "$TMP_DIR/run1.log"
fi

DUMP="$(ls -1 "$TMP_DIR"/pg-key-*.dump 2>/dev/null | head -1)"
BASELINE="$(ls -1 "$TMP_DIR"/pg-key-*.dump.counts.tsv 2>/dev/null | head -1)"
[ -n "$DUMP" ] && ok "产出备份档 $(basename "$DUMP")" || fail "未产出备份档"
[ -n "$BASELINE" ] && ok "产出行数基线 $(basename "$BASELINE")" || fail "未产出 .counts.tsv 基线"

echo ""
echo "== D. 档内容断言 =="
if [ -n "$DUMP" ]; then
	TOC="$TMP_DIR/toc.txt"
	PGPASSWORD="$PG_PASSWORD" pg_restore -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -l <"$DUMP" >"$TOC" 2>/dev/null
	if grep -qE '^[0-9]+; [0-9]+ [0-9]+ TABLE public schema_migrations ' "$TOC"; then
		ok "迁移历史 schema_migrations 在档内"
	else
		fail "迁移历史 schema_migrations 不在档内"
	fi
	if grep -qE '^[0-9]+; [0-9]+ [0-9]+ TABLE public ai_call_logs ' "$TOC"; then
		fail "排除表 ai_call_logs 仍在档内"
	else
		ok "排除表 ai_call_logs 不在档内"
	fi
	if grep -qE '^[0-9]+; [0-9]+ [0-9]+ TABLE public keep_me ' "$TOC"; then
		ok "新建业务表 keep_me 自动进档"
	else
		fail "新建业务表 keep_me 未进档"
	fi
	if grep -qE '^[0-9]+; [0-9]+ [0-9]+ TABLE public another_business_table ' "$TOC"; then
		ok "新建业务表 another_business_table 自动进档"
	else
		fail "新建业务表 another_business_table 未进档"
	fi
fi

echo ""
echo "== E. 行数基线内容断言 =="
if [ -n "$BASELINE" ]; then
	if grep -q "^keep_me	7$" "$BASELINE"; then
		ok "基线行数正确（keep_me=7）"
	else
		fail "基线行数错误："
		sed 's/^/         /' "$BASELINE"
	fi
	if grep -q "^schema_migrations	2$" "$BASELINE"; then
		ok "基线含迁移历史行数（schema_migrations=2）"
	else
		fail "基线缺 schema_migrations 行数"
	fi
	if grep -q "^ai_call_logs	" "$BASELINE"; then
		fail "基线含排除表 ai_call_logs"
	else
		ok "基线不含排除表 ai_call_logs"
	fi
fi

echo ""
echo "== F. 轮转配对（KEEP=2，连跑 3 次） =="
for i in 2 3; do
	BACKUP_DB="$SMOKE_DB" BACKUP_DIR="$TMP_DIR" BACKUP_KEEP=2 \
		BACKUP_CONTAINER="$CONTAINER" BACKUP_PG_HOST="$PG_HOST" BACKUP_PG_PORT="$PG_PORT" \
		BACKUP_PG_USER="$PG_USER" BACKUP_PG_PASSWORD="$PG_PASSWORD" \
		bash "$SCRIPT" >"$TMP_DIR/run$i.log" 2>&1 ||
		fail "第 $i 次备份非零退出"
done
DUMP_N="$(ls -1 "$TMP_DIR"/pg-key-*.dump 2>/dev/null | grep -c .)"
BASE_N="$(ls -1 "$TMP_DIR"/pg-key-*.dump.counts.tsv 2>/dev/null | grep -c .)"
if [ "$DUMP_N" = "2" ] && [ "$BASE_N" = "2" ]; then
	ok "轮转后 dump=$DUMP_N 基线=$BASE_N（KEEP=2，配对一致）"
else
	fail "轮转后配对不一致：dump=$DUMP_N 基线=$BASE_N"
fi

echo ""
echo "== 结论 =="
echo "通过 $PASS / 失败 $FAIL"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
