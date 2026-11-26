#!/usr/bin/env bash
# 主线程「该派出去就派出去」的豁免声明（router-discipline 扩展的配套）
#
# 用法：
#   router-discipline.sh --excuse "改一个 typo，单行改动，不值得派子线程"
#   router-discipline.sh --status
#   router-discipline.sh --clear
#
# 语义：写一条 per-session 豁免文件，扩展读到时静音本会话的提醒，并记一条
# policy.decision(action=bypass, reasonCode=router-excused, payload.reason=<理由>)
# 到事实库——理由进账本可回检，不是静默跳过。
#
# 与 ui-skip / doc-impact-excuse 同一纪律：显式留痕优于隐形跳过。
set -uo pipefail

BIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$BIN_DIR/../.." && pwd)"
SESSION="${PI_SESSION_ID:-default}"
DIR="$ROOT/.pi/tmp/router-discipline"
FILE="$DIR/$SESSION.json"

case "${1:---status}" in
--excuse)
	reason="${2:-}"
	if [ -z "$reason" ]; then
		echo "用法: $0 --excuse \"<理由>\"（理由不能为空——空理由不算豁免）" >&2
		exit 2
	fi
	mkdir -p "$DIR"
	python3 - "$FILE" "$reason" <<'PY'
import json, sys, datetime
path, reason = sys.argv[1], sys.argv[2]
json.dump(
    {"reason": reason, "ts": datetime.datetime.now().astimezone().isoformat(timespec="seconds")},
    open(path, "w", encoding="utf-8"), ensure_ascii=False, indent=2,
)
PY
	echo "✓ 已豁免本会话的 router-discipline 提醒"
	echo "  理由：$reason"
	echo "  （扩展下一回合读到后会静音，并把理由记入事实库）"
	;;
--status)
	if [ -f "$FILE" ]; then
		python3 -c "import json,sys;d=json.load(open(sys.argv[1]));print(f'已豁免（{d[\"ts\"]}）：{d[\"reason\"]}')" "$FILE"
	else
		echo "未豁免：本会话仍会在「读大文件 / 同文件连续 edit ≥3」时提醒一次"
	fi
	;;
--clear)
	rm -f "$FILE"
	echo "✓ 已清除豁免，提醒恢复"
	;;
*)
	echo "用法: $0 [--excuse \"<理由>\" | --status | --clear]" >&2
	exit 2
	;;
esac
