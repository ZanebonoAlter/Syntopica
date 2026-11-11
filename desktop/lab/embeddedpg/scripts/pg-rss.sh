#!/usr/bin/env bash
# pg-rss：统计 embedded PG 进程组（postmaster + 全部子进程）的 RSS。
#
# 用法：bash scripts/pg-rss.sh [pgdata 路径]
# 输出：postmaster_rss_mb / group_rss_mb / processes / data_dir
set -Eeuo pipefail

DATA_DIR="${1:-${LAB_DATA:-$PWD/_tmp/pgdata}}"
if [ ! -d "$DATA_DIR" ]; then
  echo "data dir not found: $DATA_DIR" >&2
  exit 1
fi

# postmaster：命令行里带该 data dir 的 postgres
PM="$(ps -eo pid,args | grep "[p]ostgres" | grep -F -- "-D $DATA_DIR" | awk '{print $1}' | head -1)"
if [ -z "$PM" ]; then
  echo "no embedded PG process found for $DATA_DIR" >&2
  exit 1
fi

# 子进程：ppid 链挂在 postmaster 下的全部 postgres
PIDS="$PM"
FRONTIER="$PM"
while [ -n "$FRONTIER" ]; do
  NEXT=""
  for p in $FRONTIER; do
    kids="$(ps -eo pid,ppid,args | awk -v pp="$p" '$2==pp && /postgres/ {print $1}')"
    [ -n "$kids" ] && NEXT="$NEXT $kids"
  done
  PIDS="$PIDS $NEXT"
  FRONTIER="$NEXT"
done

PM_RSS_KB="$(ps -o rss= -p "$PM" | tr -d ' ')"
TOTAL_RSS_KB="$(echo $PIDS | xargs -r ps -o rss= -p 2>/dev/null | awk '{s+=$1} END {print s+0}')"
COUNT="$(echo $PIDS | wc -w | tr -d ' ')"
# PSS：按比例分摊共享内存（shared_buffers 128MB 被多个后端映射，
# RSS 直接相加会把共享页重复计数——真实占用看 PSS）。
PSS_KB=0
for p in $PIDS; do
  v="$(awk '/^Pss:/ {s+=$2} END {print s+0}' "/proc/$p/smaps_rollup" 2>/dev/null || echo 0)"
  PSS_KB=$((PSS_KB + v))
done

printf "postmaster_rss_mb=%.1f  group_rss_mb=%.1f  group_pss_mb=%.1f  processes=%s  data_dir=%s\n" \
  "$(echo "$PM_RSS_KB" | awk '{print $1/1024}')" \
  "$(echo "$TOTAL_RSS_KB" | awk '{print $1/1024}')" \
  "$(echo "$PSS_KB" | awk '{print $1/1024}')" \
  "$COUNT" "$DATA_DIR"
