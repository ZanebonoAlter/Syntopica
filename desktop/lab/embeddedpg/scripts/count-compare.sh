#!/usr/bin/env bash
# count-compare：两库逐表行数对比（恢复完整性校验）。
#
# 用法：bash scripts/count-compare.sh <DSN_A> <label_A> <DSN_B> <label_B> [表清单文件]
#
# 表清单默认取备份档 TOC 里的 TABLE DATA 条目（= 实际进档的业务表）。
# 输出：每表 A 行数 / B 行数 / 差值；末尾给一致/不一致汇总。
set -Eeuo pipefail

DSN_A="${1:?}"; LABEL_A="${2:?}"
DSN_B="${3:?}"; LABEL_B="${4:?}"
LIST="${5:-}"
PSQL_A="${LAB_PSQL_A:-/usr/bin/psql}"
PSQL_B="${LAB_PSQL_B:-/usr/bin/psql}"

if [ -z "$LIST" ]; then
  echo "需要表清单文件（pg_restore -l 输出）" >&2
  exit 2
fi

TABLES="$(grep "TABLE DATA" "$LIST" | sed -E 's/.*TABLE DATA public ([^ ]+) .*/\1/')"
printf "%-42s %12s %12s %10s\n" "table" "$LABEL_A" "$LABEL_B" "delta"
MISMATCH=0
TOTAL=0
for t in $TABLES; do
  a="$("$PSQL_A" "$DSN_A" -X -Atc "select count(*) from public.\"$t\"" 2>/dev/null || echo ERR)"
  b="$("$PSQL_B" "$DSN_B" -X -Atc "select count(*) from public.\"$t\"" 2>/dev/null || echo ERR)"
  TOTAL=$((TOTAL+1))
  if [ "$a" != "$b" ]; then
    MISMATCH=$((MISMATCH+1))
    printf "%-42s %12s %12s %10s  <-- 不一致\n" "$t" "$a" "$b" "$((a-b))"
  else
    printf "%-42s %12s %12s %10s\n" "$t" "$a" "$b" "0"
  fi
done
echo "----"
echo "tables_total=$TOTAL mismatched=$MISMATCH"
