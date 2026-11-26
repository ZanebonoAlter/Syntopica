#!/usr/bin/env bash
# install-pgvector：把 pgvector 扩展三件套装进 embedded PG 的安装目录。
#
# 用法：
#   bash scripts/install-pgvector.sh <docker容器名|本地目录> [安装目录]
#
#   安装目录默认 $LAB_INSTALL（缺省 <lab>/_tmp/install/postgresql-18.6.1）
#
# 来源两种：
#   1) docker 容器：从 `pgvector/pgvector:pg18-trixie` 镜像里抠已编译好的
#      vector.so + vector.control + vector--*.sql（arm64 原生，无需本地编译）。
#   2) 本地目录：已经是 PGDG 装好的 <pgroot>（含 lib/ 与 share/extension/）。
#
# 三件套落位（PG 标准扩展机制）：
#   <install>/lib/vector.so            ← $libdir/vector
#   <install>/share/extension/vector.control
#   <install>/share/extension/vector--<ver>.sql
set -Eeuo pipefail

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL="${2:-${LAB_INSTALL:-$LAB_DIR/_tmp/install/postgresql-18.6.1}}"
SRC="${1:?usage: install-pgvector.sh <docker容器名|本地目录> [安装目录]}"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

if [ ! -d "$INSTALL/lib" ] || [ ! -d "$INSTALL/share/extension" ]; then
  log "ERROR: 安装目录不像 PG 运行时（缺 lib/ 或 share/extension/）: $INSTALL"
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if docker ps -a --format '{{.Names}}' | grep -qx "$SRC"; then
  log "从容器 $SRC 抽取 pgvector ..."
  # 容器内 PG 大版本目录（pg18 → 18）
  PGVER="$(docker exec "$SRC" bash -c "ls -d /usr/lib/postgresql/*/lib/vector.so | head -1" | sed -E 's#.*/postgresql/([0-9]+)/.*#\1#')"
  if [ -z "$PGVER" ]; then
    log "ERROR: 容器内找不到 vector.so"
    exit 1
  fi
  log "容器内 PG 主版本目录: $PGVER"
  docker cp "$SRC:/usr/lib/postgresql/$PGVER/lib/vector.so" "$TMP/vector.so" >/dev/null
  docker cp "$SRC:/usr/share/postgresql/$PGVER/extension/vector.control" "$TMP/vector.control" >/dev/null
  # docker cp 不支持通配——逐个取
  for f in $(docker exec "$SRC" bash -c "ls /usr/share/postgresql/$PGVER/extension/vector--*.sql"); do
    base="$(basename "$f")"
    docker cp "$SRC:$f" "$TMP/$base" >/dev/null
  done
  # 容器内 PG 小版本 → 记进 manifest，便于事后判断 ABI 兼容
  docker exec "$SRC" postgres --version > "$TMP/pg_version.txt" 2>&1 || true
elif docker image inspect "$SRC" >/dev/null 2>&1; then
  log "从镜像 $SRC 抽取 pgvector ..."
  docker run --rm "$SRC" sh -c 'cd / && tar -cf - usr/lib/postgresql/18/lib/vector.so usr/share/postgresql/18/extension/vector.control usr/share/postgresql/18/extension/vector--*.sql' > "$TMP/v.tar" 2>/dev/null
  tar -xf "$TMP/v.tar" -C "$TMP"
  mv "$TMP/usr/lib/postgresql/18/lib/vector.so" "$TMP/vector.so"
  mv "$TMP/usr/share/postgresql/18/extension/vector.control" "$TMP/vector.control"
  mv "$TMP/usr/share/postgresql/18/extension/"vector--*.sql "$TMP/" 2>/dev/null || true
  rm -rf "$TMP/usr"
  docker run --rm "$SRC" postgres --version > "$TMP/pg_version.txt" 2>&1 || true
else
  log "从本地目录 $SRC 抽取 pgvector ..."
  [ -d "$SRC" ] || { log "ERROR: 目录不存在: $SRC"; exit 1; }
  find "$SRC" -name 'vector.so' -exec cp {} "$TMP/vector.so" \; 2>/dev/null || true
  find "$SRC" -name 'vector.control' -exec cp {} "$TMP/vector.control" \; 2>/dev/null || true
  find "$SRC" -name 'vector--*.sql' -exec cp {} "$TMP/" \; 2>/dev/null || true
fi

[ -s "$TMP/vector.so" ] || { log "ERROR: 没拿到 vector.so"; exit 1; }
[ -s "$TMP/vector.control" ] || { log "ERROR: 没拿到 vector.control"; exit 1; }

install -m 0755 "$TMP/vector.so" "$INSTALL/lib/vector.so"
install -m 0644 "$TMP/vector.control" "$INSTALL/share/extension/vector.control"
shopt -s nullglob
for f in "$TMP"/vector--*.sql; do
  install -m 0644 "$f" "$INSTALL/share/extension/$(basename "$f")"
done
shopt -u nullglob

VER="$(grep -E "^default_version" "$TMP/vector.control" | cut -d"'" -f2)"
log "pgvector $VER 已装入 $INSTALL"
log "  vector.so            $(stat -c%s "$INSTALL/lib/vector.so") bytes"
log "  share/extension      $(ls "$INSTALL/share/extension" | grep -c '^vector') 个文件"
[ -f "$TMP/pg_version.txt" ] && log "  来源 PG 版本         $(cat "$TMP/pg_version.txt" | head -1)"
