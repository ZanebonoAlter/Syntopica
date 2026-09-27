#!/usr/bin/env bash
# Syntopica 同源入口（nginx）幂等安装脚本
#
# 用法（需要 root，因为要写 /etc/nginx 并 reload 服务）：
#   sudo bash deploy/same-origin/install-nginx.sh          # dev 模式（反代 :3000，保留 HMR）
#   sudo bash deploy/same-origin/install-nginx.sh static   # 静态产物（托管 /srv/www）
#
# 做的事：装 nginx 包（缺失时）→ 铺 conf.d/syntopica.conf → 停用 Debian 默认站点
#        → nginx -t 校验（失败自动回滚旧配置）→ enable + reload
# 幂等：重复执行等价；切变体就是重跑一次带不同参数。
set -euo pipefail

VARIANT="${1:-dev}"
case "$VARIANT" in
	dev)    SRC="nginx.conf" ;;
	static) SRC="nginx.static.conf" ;;
	*) echo "用法: $0 [dev|static]" >&2; exit 2 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
	echo "需要 root：sudo bash $0 $VARIANT" >&2
	exit 1
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_PATH="$DIR/$SRC"
DEST="/etc/nginx/conf.d/syntopica.conf"

[ -f "$SRC_PATH" ] || { echo "找不到配置模板 $SRC_PATH" >&2; exit 1; }

# 1. nginx 包（缺失时才装，避免动已装版本）
if ! command -v nginx >/dev/null 2>&1; then
	echo "[1/5] 安装 nginx ..."
	apt-get update -qq
	DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx
else
	echo "[1/5] nginx 已安装：$(nginx -v 2>&1)"
fi

# 2. 铺配置（先备份旧文件，校验失败好回滚）
echo "[2/5] 写入 $DEST （变体：$VARIANT / $SRC）"
BACKUP=""
if [ -f "$DEST" ]; then
	BACKUP="$(mktemp /tmp/syntopica-nginx.XXXXXX)"
	cp -a "$DEST" "$BACKUP"
fi
install -m 0644 "$SRC_PATH" "$DEST"

# 3. 停用 Debian 默认站点：它同样声明 default_server，与我们的 :80 冲突
if [ -e /etc/nginx/sites-enabled/default ]; then
	echo "[3/5] 停用 Debian 默认站点（恢复：ln -s /etc/nginx/sites-available/default /etc/nginx/sites-enabled/default）"
	rm -f /etc/nginx/sites-enabled/default
else
	echo "[3/5] Debian 默认站点已停用，跳过"
fi

# 4. 校验，失败回滚
echo "[4/5] nginx -t"
if ! nginx -t; then
	echo "配置校验失败，回滚 $DEST" >&2
	if [ -n "$BACKUP" ]; then
		cp -a "$BACKUP" "$DEST"
	else
		rm -f "$DEST"
	fi
	rm -f "$BACKUP"
	exit 1
fi
[ -n "$BACKUP" ] && rm -f "$BACKUP"

# 5. 生效
echo "[5/5] enable + reload"
systemctl enable nginx >/dev/null 2>&1 || true
systemctl reload nginx 2>/dev/null || systemctl restart nginx

echo
echo "完成。入口：http://$(hostname -I | awk '{print $1}')/  （同源，无需 CORS / apiBase 配置）"
if [ "$VARIANT" = "dev" ]; then
	echo "提示：前端必须以相对 base 重启才生效 —— cd front && NUXT_PUBLIC_API_BASE=/api pnpm dev --host"
else
	echo "提示：静态产物需先铺到 /srv/www（见 $SRC 头部注释）"
fi
