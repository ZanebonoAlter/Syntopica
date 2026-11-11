#!/usr/bin/env bash
# restore-key-tables.smoke.sh — 恢复脚本冒烟（change: fix-disaster-recovery-channel）
#
# 覆盖场景：
#   - 参数校验（缺 --dump / 档不存在 → 非零退出）
#   - 落库闸门：目标库为生产库时拒绝执行（--allow-prod 未给 / 二次确认不通过）
#   - 扩展预装：扩展缺失时明确报错且未进入 restore（不留半恢复库）
#   - TOC 过滤：过滤后无 SCHEMA public 冲突（恢复成功即证明）
#   - count 校验：基线一致 → 零退出；基线不一致 → 列出差异表并非零退出
#   - 迁移状态对账：检出被排除表导致的缺口；表集合一致时缺口列表为空
#   - 报告输出：含档来源 / 目标库 / count 比对 / 迁移对账 / 耗时
#   - 默认恢复到新建独立库
#
# 全程在一次性测试库 + 临时目录上跑。生产库名通过 RESTORE_PROD_DB 指向测试库，
# 因此**永不**碰真库 syntopica。
# 退出码：0 通过；1 有失败。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

CONTAINER="${RESTORE_CONTAINER:-syntopica-postgres}"
PG_USER="${RESTORE_PG_USER:-postgres}"
PG_PASSWORD="${RESTORE_PG_PASSWORD:-postgres}"
PG_HOST="${RESTORE_PG_HOST:-127.0.0.1}"
PG_PORT="${RESTORE_PG_PORT:-5432}"
SRC_DB="syntopica_smoke_src_$$"
TMP_DIR="$(mktemp -d)"
BACKUP_SCRIPT="scripts/db/backup-key-tables.sh"
RESTORE_SCRIPT="scripts/db/restore-key-tables.sh"

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
	for db in "$SRC_DB" "$SRC_DB"_restore_* "$SRC_DB"_target_*; do
		PGPASSWORD="$PG_PASSWORD" dropdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" --if-exists "$db" >/dev/null 2>&1 || true
	done
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT

psql_src() {
	PGPASSWORD="$PG_PASSWORD" psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$SRC_DB" -v ON_ERROR_STOP=1 "$@"
}

db_exists() {
	PGPASSWORD="$PG_PASSWORD" psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname = '$1'" 2>/dev/null | grep -q 1
}

row_count() {
	PGPASSWORD="$PG_PASSWORD" psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$1" -tAc "SELECT count(*) FROM $2" 2>/dev/null | tr -d '[:space:]'
}

echo "== A. 准备一次性源库 + 备份档 =="
PGPASSWORD="$PG_PASSWORD" dropdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" --if-exists "$SRC_DB" >/dev/null 2>&1 || true
if ! PGPASSWORD="$PG_PASSWORD" createdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" "$SRC_DB" >/dev/null 2>&1; then
	fail "无法创建源测试库 $SRC_DB"
	exit 1
fi
ok "源测试库 $SRC_DB 已创建"

psql_src -q <<'SQL' >/dev/null
CREATE TABLE keep_me (id bigserial PRIMARY KEY, name text);
CREATE TABLE another_business_table (id bigserial PRIMARY KEY);
CREATE TABLE ai_call_logs (id bigserial PRIMARY KEY, payload text);
CREATE TABLE schema_migrations (version text PRIMARY KEY);
INSERT INTO keep_me (name) SELECT 'row-' || g FROM generate_series(1, 7) g;
INSERT INTO another_business_table SELECT g FROM generate_series(1, 3) g;
INSERT INTO ai_call_logs (payload) SELECT 'log' FROM generate_series(1, 5) g;
INSERT INTO schema_migrations (version) VALUES ('20260403_0001'), ('20260403_0003');
SQL
ok "源库表就绪（keep_me / another_business_table / ai_call_logs / schema_migrations）"

BACKUP_DB="$SRC_DB" BACKUP_DIR="$TMP_DIR" BACKUP_KEEP=7 \
	BACKUP_CONTAINER="$CONTAINER" BACKUP_PG_HOST="$PG_HOST" BACKUP_PG_PORT="$PG_PORT" \
	BACKUP_PG_USER="$PG_USER" BACKUP_PG_PASSWORD="$PG_PASSWORD" \
	bash "$BACKUP_SCRIPT" >"$TMP_DIR/backup.log" 2>&1 ||
	{
		fail "备份脚本失败"
		sed 's/^/         /' "$TMP_DIR/backup.log"
		exit 1
	}
ARCHIVE="$(ls -1 "$TMP_DIR"/pg-key-*.dump 2>/dev/null | head -1)"
BASELINE="$(ls -1 "$TMP_DIR"/pg-key-*.dump.counts.tsv 2>/dev/null | head -1)"
[ -n "$ARCHIVE" ] && ok "备份档就绪：$(basename "$ARCHIVE")" || fail "未产出备份档"
[ -n "$BASELINE" ] && ok "行数基线就绪" || fail "未产出 .counts.tsv"

echo ""
echo "== B. 参数校验分支 =="
bash "$RESTORE_SCRIPT" >/dev/null 2>&1
[ $? -ne 0 ] && ok "缺 --dump → 非零退出" || fail "缺 --dump 未被拒绝"
bash "$RESTORE_SCRIPT" --dump "$TMP_DIR/does-not-exist.dump" >/dev/null 2>&1
[ $? -ne 0 ] && ok "档不存在 → 非零退出" || fail "档不存在未被拒绝"
bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --bogus-flag >/dev/null 2>&1
[ $? -ne 0 ] && ok "未知参数 → 非零退出" || fail "未知参数未被拒绝"

echo ""
echo "== C. 落库闸门（生产库名 = 源测试库，绝不碰真库） =="
BEFORE_KEEP_ME="$(row_count "$SRC_DB" keep_me)"
BEFORE_LOGS="$(row_count "$SRC_DB" ai_call_logs)"
RESTORE_PROD_DB="$SRC_DB" bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$SRC_DB" >"$TMP_DIR/gate.log" 2>&1
GATE_RC=$?
if [ "$GATE_RC" -ne 0 ]; then
	ok "指定生产库名（无 --allow-prod）→ 非零退出（rc=$GATE_RC）"
else
	fail "指定生产库名竟成功执行"
fi
grep -q "生产库" "$TMP_DIR/gate.log" && ok "拒绝原因写明「生产库」" || fail "拒绝原因未说明"

echo "yes" | RESTORE_PROD_DB="$SRC_DB" bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$SRC_DB" --allow-prod >"$TMP_DIR/gate2.log" 2>&1
GATE2_RC=$?
if [ "$GATE2_RC" -ne 0 ]; then
	ok "--allow-prod 但二次确认输入错误 → 非零退出（rc=$GATE2_RC）"
else
	fail "二次确认未生效"
fi
[ "$(row_count "$SRC_DB" keep_me)" = "$BEFORE_KEEP_ME" ] &&
	[ "$(row_count "$SRC_DB" ai_call_logs)" = "$BEFORE_LOGS" ] &&
	ok "闸门拒绝后生产库行数不变（keep_me=$BEFORE_KEEP_ME / ai_call_logs=$BEFORE_LOGS）" ||
	fail "闸门拒绝后生产库行数被改动"

echo ""
echo "== D. 扩展预装失败 → restore 之前报错、不留半恢复库 =="
EXT_TARGET="${SRC_DB}_target_ext"
RESTORE_PROD_DB="$SRC_DB" RESTORE_EXTENSIONS="vector bogus_ext_syntopica_probe" \
	bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$EXT_TARGET" >"$TMP_DIR/ext.log" 2>&1
EXT_RC=$?
if [ "$EXT_RC" -ne 0 ]; then
	ok "扩展缺失 → 非零退出（rc=$EXT_RC）"
else
	fail "扩展缺失竟成功"
fi
grep -qi "扩展" "$TMP_DIR/ext.log" && ok "报错信息点名缺失扩展" || fail "报错未提扩展"
if db_exists "$EXT_TARGET"; then
	fail "扩展失败后目标库被留下（半恢复库）"
else
	ok "扩展失败后目标库已清理（无半恢复库）"
fi

echo ""
echo "== E. 正常恢复（默认新建独立库） =="
RESTORE_PROD_DB="$SRC_DB" bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" \
	--report "$TMP_DIR/report.md" >"$TMP_DIR/restore.log" 2>&1
RC=$?
if [ "$RC" -eq 0 ]; then
	ok "恢复脚本退出码 0"
else
	fail "恢复脚本退出码 $RC"
	sed 's/^/         /' "$TMP_DIR/restore.log" | tail -20
fi
grep -q "count 完整性校验" "$TMP_DIR/report.md" && ok "报告含 count 校验小节" || fail "报告缺 count 校验小节"
grep -q "迁移状态对账" "$TMP_DIR/report.md" && ok "报告含迁移对账小节" || fail "报告缺迁移对账小节"
grep -q "耗时" "$TMP_DIR/report.md" && ok "报告含耗时小节" || fail "报告缺耗时小节"
grep -q "档来源" "$TMP_DIR/report.md" && ok "报告含档来源" || fail "报告缺档来源"
grep -q "目标库" "$TMP_DIR/report.md" && ok "报告含目标库" || fail "报告缺目标库"
grep -q "SCHEMA public 条目已剔除" "$TMP_DIR/report.md" && ok "报告记录了 TOC 过滤" || fail "报告未记录 TOC 过滤"

echo ""
echo "== F. count 校验：一致 / 不一致 =="
TARGET_OK="${SRC_DB}_target_ok"
RESTORE_PROD_DB="$SRC_DB" bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$TARGET_OK" --keep \
	--report "$TMP_DIR/report-ok.md" >"$TMP_DIR/restore-ok.log" 2>&1
RC=$?
[ "$RC" -eq 0 ] && ok "基线一致 → 零退出" || fail "基线一致却非零退出（rc=$RC）"
grep -q "通过（" "$TMP_DIR/report-ok.md" && ok "报告标记 count 校验通过" || fail "报告未标记通过"
[ "$(row_count "$TARGET_OK" keep_me)" = "7" ] && ok "恢复库 keep_me 行数正确（7）" || fail "恢复库 keep_me 行数不对"

# 篡改基线 → 制造不一致
BAD_DIR="$TMP_DIR/bad"
mkdir -p "$BAD_DIR"
cp "$ARCHIVE" "$BAD_DIR/"
cp "$BASELINE" "$BAD_DIR/"
BAD_BASELINE="$BAD_DIR/$(basename "$BASELINE")"
sed -i 's/^keep_me\t7$/keep_me\t99/' "$BAD_BASELINE"
TARGET_BAD="${SRC_DB}_target_bad"
RESTORE_PROD_DB="$SRC_DB" bash "$RESTORE_SCRIPT" --dump "$BAD_DIR/$(basename "$ARCHIVE")" \
	--target-db "$TARGET_BAD" --keep --report "$TMP_DIR/report-bad.md" >"$TMP_DIR/restore-bad.log" 2>&1
RC=$?
if [ "$RC" -ne 0 ]; then
	ok "基线不一致 → 非零退出（rc=$RC）"
else
	fail "基线不一致却零退出"
fi
grep -q "keep_me" "$TMP_DIR/report-bad.md" && ok "报告列出差异表 keep_me" || fail "报告未列出差异表"
grep -q "基线 99" "$TMP_DIR/report-bad.md" && ok "报告同时给出基线值与实际值" || fail "报告未给出基线/实际值"

echo ""
echo "== G. 迁移状态对账 =="
# 缺口判定 = 「已应用迁移（恢复库 schema_migrations 里的版本）触及的表」不在恢复库。
# 用迷你迁移源码把判定条件固定住：版本取档内 schema_migrations 已有的 20260403_0001。
cat >"$TMP_DIR/gap-migrations.go" <<'GO'
package fake

// 迷你迁移源码：触及**不在档内**的表 ai_call_logs（被备份排除清单挡掉）→ 应报缺口。
func migrations() []any {
	return []any{
		{
			Version:     "20260403_0001",
			Description: "probe migration touching an excluded table",
			Up: func(db *gorm.DB) error {
				return db.Exec("ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS probe int").Error
			},
		},
	}
}
GO
TARGET_GAP="${SRC_DB}_target_gap"
RESTORE_PROD_DB="$SRC_DB" RESTORE_MIGRATION_SRC="$TMP_DIR/gap-migrations.go" \
	bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$TARGET_GAP" --keep \
	--report "$TMP_DIR/report-gap.md" >"$TMP_DIR/restore-gap.log" 2>&1
RC=$?
[ "$RC" -eq 0 ] && ok "检出缺口时仍零退出（缺口不影响退出码）" || fail "检出缺口却非零退出（rc=$RC）"
grep -q "ai_call_logs" "$TMP_DIR/report-gap.md" && ok "检出被排除表导致的迁移缺口（ai_call_logs）" ||
	fail "未检出 ai_call_logs 缺口"
grep -qE "缺口（迁移已应用、但表不在档内）：[1-9]" "$TMP_DIR/report-gap.md" &&
	ok "缺口计数非零" || fail "缺口计数为零"

# 用一份只触及「档内表」的迷你迁移源码 → 缺口列表应为空
cat >"$TMP_DIR/mini-migrations.go" <<'GO'
package fake

// 迷你迁移源码：只触及档内存在的表 keep_me → 对账不应报缺口。
func migrations() []any {
	return []any{
		{
			Version:     "20260403_0001",
			Description: "probe migration touching keep_me",
			Up: func(db *gorm.DB) error {
				return db.Exec("ALTER TABLE keep_me ADD COLUMN IF NOT EXISTS probe int").Error
			},
		},
	}
}
GO
TARGET_NOGAP="${SRC_DB}_target_nogap"
RESTORE_PROD_DB="$SRC_DB" RESTORE_MIGRATION_SRC="$TMP_DIR/mini-migrations.go" \
	bash "$RESTORE_SCRIPT" --dump "$ARCHIVE" --target-db "$TARGET_NOGAP" --keep \
	--report "$TMP_DIR/report-nogap.md" >"$TMP_DIR/restore-nogap.log" 2>&1
RC=$?
[ "$RC" -eq 0 ] && ok "无缺口场景 → 零退出" || fail "无缺口场景却非零退出（rc=$RC）"
grep -q "缺口（迁移已应用、但表不在档内）：0" "$TMP_DIR/report-nogap.md" &&
	ok "无缺口时缺口列表为空" || fail "无缺口时缺口列表非空"

echo ""
echo "== H. 收尾核对 =="
[ "$(row_count "$SRC_DB" keep_me)" = "$BEFORE_KEEP_ME" ] &&
	[ "$(row_count "$SRC_DB" ai_call_logs)" = "$BEFORE_LOGS" ] &&
	ok "演练全程源库（=闸门视角的生产库）行数不变" || fail "源库行数被改动"

echo ""
echo "== 结论 =="
echo "通过 $PASS / 失败 $FAIL"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
