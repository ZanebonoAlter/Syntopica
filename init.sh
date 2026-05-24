#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# ── Colors ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ── Helpers ─────────────────────────────────────────────────────────────────
info()  { printf "${BLUE}ℹ${NC} %s\n" "$*"; }
ok()    { printf "${GREEN}✔${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}⚠${NC} %s\n" "$*"; }
fail()  { printf "${RED}✖${NC} %s\n" "$*" >&2; exit 1; }
step()  { printf "\n${BOLD}${CYAN}── %s ──${NC}\n" "$*"; }
ask()   {
  local prompt="$1" default="${2:-}"
  if [ -n "$default" ]; then
    printf "${YELLOW}?${NC} %s [${default}]: " "$prompt"
  else
    printf "${YELLOW}?${NC} %s: " "$prompt"
  fi
  read -r answer
  answer="${answer:-$default}"
}

ask_yn() {
  local prompt="$1" default="${2:-Y}"
  local yn
  while true; do
    printf "${YELLOW}?${NC} %s [%s]: " "$prompt" "$default"
    read -r yn
    yn="${yn:-$default}"
    case "$yn" in
      [Yy]|[Yy][Ee][Ss]) return 0 ;;
      [Nn]|[Nn][Oo]) return 1 ;;
      *) warn "请输入 Y 或 n" ;;
    esac
  done
}

curl_silent() {
  curl -sS -o /dev/null -w "%{http_code}" "$@" 2>/dev/null || echo "000"
}

separator() { printf "${DIM}─────────────────────────────────────────────────────${NC}\n"; }

# ── State variables ─────────────────────────────────────────────────────────
PORT="5000"
POSTGRES_PORT="5432"
POSTGRES_DB="syntopica"
POSTGRES_USER="postgres"
POSTGRES_PASSWORD="postgres"
TZ="Asia/Shanghai"

AI_MODE=""
AI_IP=""
TEXT_PORT=""
EMBED_PORT=""
TEXT_MODEL=""
EMBED_MODEL=""
AI_PROVIDER_TYPE=""
AI_API_KEY=""
AI_BASE_URL=""
EMBED_BASE_URL=""

FIRECRAWL_MODE=""
FIRECRAWL_API_URL=""
FIRECRAWL_API_KEY=""

# ════════════════════════════════════════════════════════════════════════════
# 阶段 1：核心启动
# ════════════════════════════════════════════════════════════════════════════
phase1() {
  step "阶段 1：核心启动"

  check_docker

  collect_config

  merge_env

  start_services

  wait_healthy
}

check_docker() {
  info "检查 Docker..."
  if ! command -v docker &>/dev/null; then
    fail "Docker 未安装。请先安装 Docker Desktop。\n  https://www.docker.com/products/docker-desktop/"
  fi

  if ! docker compose version &>/dev/null 2>&1; then
    if ! docker-compose version &>/dev/null 2>&1; then
      fail "docker compose 插件未找到。请更新 Docker Desktop 或安装 docker-compose。"
    fi
    DOCKER_COMPOSE="docker-compose"
  else
    DOCKER_COMPOSE="docker compose"
  fi
  ok "Docker 和 $DOCKER_COMPOSE 可用"
}

collect_config() {
  step "配置"
  echo "按 Enter 接受默认值。\n"

  ask "服务端口" "$PORT"; PORT="$answer"
  ask "PostgreSQL 端口" "$POSTGRES_PORT"; POSTGRES_PORT="$answer"
  ask "PostgreSQL 数据库名" "$POSTGRES_DB"; POSTGRES_DB="$answer"
  ask "PostgreSQL 用户名" "$POSTGRES_USER"; POSTGRES_USER="$answer"
  ask "PostgreSQL 密码" "$POSTGRES_PASSWORD"; POSTGRES_PASSWORD="$answer"
  ask "时区" "$TZ"; TZ="$answer"
}

merge_env() {
  step "生成 .env"

  local env_file="$SCRIPT_DIR/.env"
  local example_file="$SCRIPT_DIR/.env.example"

  if [ ! -f "$example_file" ]; then
    warn ".env.example 未找到，创建最小 .env"
    cat > "$env_file" <<EOF
PORT=$PORT
POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_PORT=$POSTGRES_PORT
TZ=$TZ
EOF
    ok ".env 已创建"
    return
  fi

  cp "$example_file" "$env_file.tmp"

  local -A defaults
  defaults[PORT]="$PORT"
  defaults[POSTGRES_DB]="$POSTGRES_DB"
  defaults[POSTGRES_USER]="$POSTGRES_USER"
  defaults[POSTGRES_PASSWORD]="$POSTGRES_PASSWORD"
  defaults[POSTGRES_PORT]="$POSTGRES_PORT"
  defaults[TZ]="$TZ"

  for key in "${!defaults[@]}"; do
    if grep -q "^${key}=" "$env_file" 2>/dev/null; then
      local existing
      existing=$(grep "^${key}=" "$env_file" | head -1 | cut -d'=' -f2-)
      if [ -n "$existing" ]; then
        sed -i "s|^${key}=.*|${key}=${existing}|" "$env_file.tmp"
        eval "$key=\"\$existing\""
      else
        sed -i "s|^${key}=.*|${key}=${defaults[$key]}|" "$env_file.tmp"
      fi
    fi
  done

  mv "$env_file.tmp" "$env_file"
  ok ".env 已更新（已有值已保留）"
}

start_services() {
  step "启动服务"

  info "运行 $DOCKER_COMPOSE up -d..."
  $DOCKER_COMPOSE up -d 2>&1 || fail "Docker 服务启动失败"

  ok "Docker 服务已启动"
}

wait_healthy() {
  step "等待服务就绪"

  # 等待 PostgreSQL
  info "等待 PostgreSQL..."
  local retries=30
  while [ $retries -gt 0 ]; do
    if $DOCKER_COMPOSE exec -T postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" &>/dev/null; then
      break
    fi
    retries=$((retries - 1))
    sleep 2
  done
  if [ $retries -eq 0 ]; then
    fail "PostgreSQL 未能在规定时间内就绪"
  fi
  ok "PostgreSQL 已就绪"

  # 等待后端健康检查
  info "等待后端 /health..."
  retries=60
  local code
  while [ $retries -gt 0 ]; do
    code=$(curl_silent "http://localhost:${PORT}/health")
    if [ "$code" = "200" ]; then
      break
    fi
    retries=$((retries - 1))
    sleep 3
  done
  if [ $retries -eq 0 ]; then
    fail "后端 /health 未返回 200（最后状态码: $code）"
  fi
  ok "后端服务健康"
}

# ════════════════════════════════════════════════════════════════════════════
# 阶段 2：可选配置
# ════════════════════════════════════════════════════════════════════════════
phase2() {
  step "阶段 2：可选配置"

  detect_ip
  ask_ai_mode
  if [ "$AI_MODE" != "skip" ]; then
    health_check_ai
  fi

  ask_firecrawl_mode
}

# 5.3 IP 自动检测
detect_ip() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"

  case "$os" in
    linux*)
      AI_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
      ;;
    darwin*)
      AI_IP=$(ipconfig getifaddr en0 2>/dev/null || echo "")
      ;;
    mingw*|msys*|cygwin*)
      AI_IP=$(ipconfig 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)
      ;;
    *)
      AI_IP=""
      ;;
  esac

  if [ -z "$AI_IP" ]; then
    AI_IP="localhost"
    warn "无法自动检测本机 IP，使用 localhost（Docker 内可能无法访问）"
  else
    info "检测到本机 IP: $AI_IP"
  fi
}

# 5.2 AI 模式选择
ask_ai_mode() {
  step "AI 模型配置"
  echo "如何连接 AI 模型？"
  echo ""
  echo "  1) Ollama"
  echo "  2) llama.cpp（本地服务）"
  echo "  3) 远程 API（OpenAI 兼容）"
  echo "  4) 暂时跳过"
  echo ""

  local choice
  while true; do
    ask "请选择 [1-4]" "4"
    choice="$answer"
    case "$choice" in
      1) AI_MODE="ollama"; setup_ollama; break ;;
      2) AI_MODE="llamacpp"; setup_llamacpp; break ;;
      3) AI_MODE="remote"; setup_remote; break ;;
      4) AI_MODE="skip"; info "已跳过 AI 配置"; break ;;
      *) warn "请输入 1、2、3 或 4" ;;
    esac
  done
}

# 5.4 Ollama 配置
setup_ollama() {
  AI_PROVIDER_TYPE="ollama"
  TEXT_PORT="11434"
  EMBED_PORT="11434"

  ask "AI 服务 IP 地址" "$AI_IP"; AI_IP="$answer"
  ask "Ollama 端口" "$TEXT_PORT"; TEXT_PORT="$answer"
  EMBED_PORT="$TEXT_PORT"

  AI_BASE_URL="http://${AI_IP}:${TEXT_PORT}/v1"
  EMBED_BASE_URL="$AI_BASE_URL"

  ask "文本模型名称" "qwen3:8b"; TEXT_MODEL="$answer"
  ask "嵌入模型名称" "nomic-embed-text"; EMBED_MODEL="$answer"
  AI_API_KEY=""
}

# 5.5 llama.cpp 配置
setup_llamacpp() {
  AI_PROVIDER_TYPE="openai_compatible"
  TEXT_PORT="8080"
  EMBED_PORT="8081"

  ask "AI 服务 IP 地址" "$AI_IP"; AI_IP="$answer"
  ask "文本服务端口" "$TEXT_PORT"; TEXT_PORT="$answer"
  ask "嵌入服务端口" "$EMBED_PORT"; EMBED_PORT="$answer"

  AI_BASE_URL="http://${AI_IP}:${TEXT_PORT}/v1"
  EMBED_BASE_URL="http://${AI_IP}:${EMBED_PORT}/v1"

  TEXT_MODEL="loaded-model"
  EMBED_MODEL="loaded-model"
  AI_API_KEY="sk-local"
}

# 5.6 远程 API 配置
setup_remote() {
  AI_PROVIDER_TYPE="openai_compatible"

  ask "API 地址" "https://api.openai.com/v1"; AI_BASE_URL="$answer"
  EMBED_BASE_URL="$AI_BASE_URL"
  ask "API 密钥"; AI_API_KEY="$answer"
  ask "文本模型名称" "gpt-4o-mini"; TEXT_MODEL="$answer"
  ask "嵌入模型名称" "text-embedding-3-small"; EMBED_MODEL="$answer"
}

# 5.7 AI 健康检测
health_check_ai() {
  step "AI 健康检查"

  if [ "$AI_MODE" = "remote" ]; then
    info "远程 API 模式，跳过健康检查"
    return
  fi

  local health_url="http://${AI_IP}:${TEXT_PORT}/v1/chat/completions"
  info "测试 AI 服务 ${AI_IP}:${TEXT_PORT}..."

  local retries=3
  while [ $retries -gt 0 ]; do
    local response
    response=$(curl -s -w "\n%{http_code}" \
      "$health_url" \
      -H "Content-Type: application/json" \
      -d "{\"model\":\"${TEXT_MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}],\"max_tokens\":5}" \
      --max-time 30 2>/dev/null || echo -e "\n000")

    local code
    code=$(echo "$response" | tail -1)

    if [ "$code" = "200" ]; then
      ok "AI 服务连接正常"
      return
    fi

    retries=$((retries - 1))
    if [ $retries -gt 0 ]; then
      warn "AI 服务未响应 (HTTP $code)，${retries} 次重试剩余..."
      sleep 5
    fi
  done

  warn "无法连接 AI 服务: $health_url"
  if ! ask_yn "是否继续？（可稍后在 Web UI 中配置）" "Y"; then
    AI_MODE="skip"
    info "已跳过 AI 配置，种子数据不会写入"
  fi
}

# 5.8 Firecrawl 模式
ask_firecrawl_mode() {
  step "Firecrawl 配置"
  echo "如何处理全文爬取？"
  echo ""
  echo "  1) 自部署 Firecrawl (docker-compose.firecrawl.yml)"
  echo "  2) 云端 API（使用 Firecrawl 云服务）"
  echo "  3) 跳过 Firecrawl 配置"
  echo ""

  local choice
  while true; do
    ask "请选择 [1-3]" "3"
    choice="$answer"
    case "$choice" in
      1) FIRECRAWL_MODE="self"; setup_firecrawl_self; break ;;
      2) FIRECRAWL_MODE="cloud"; setup_firecrawl_cloud; break ;;
      3) FIRECRAWL_MODE="skip"; info "已跳过 Firecrawl 配置"; break ;;
      *) warn "请输入 1、2 或 3" ;;
    esac
  done
}

setup_firecrawl_self() {
  if [ -f "$SCRIPT_DIR/docker-compose.firecrawl.yml" ]; then
    info "启动 Firecrawl 服务..."
    if $DOCKER_COMPOSE -f docker-compose.firecrawl.yml up -d 2>&1; then
      ok "Firecrawl 已启动"
    else
      warn "Firecrawl 启动失败"
      warn "可手动启动: docker compose -f docker-compose.firecrawl.yml up -d"
    fi
    FIRECRAWL_API_URL="http://firecrawl:3002"
  else
    warn "docker-compose.firecrawl.yml 未找到"
    warn "请先创建该文件，参见 docs/reference/deployment.md"
    FIRECRAWL_API_URL="http://firecrawl:3002"
  fi
}

setup_firecrawl_cloud() {
  ask "Firecrawl 云端 API 地址" "https://api.firecrawl.dev/v1"; FIRECRAWL_API_URL="$answer"
  ask "Firecrawl API 密钥"; FIRECRAWL_API_KEY="$answer"
  ok "Firecrawl 云端已配置"
}

# ════════════════════════════════════════════════════════════════════════════
# 阶段 3：确认与执行
# ════════════════════════════════════════════════════════════════════════════
phase3() {
  step "阶段 3：确认"

  print_summary

  if ! ask_yn "确认执行？" "Y"; then
    info "已取消。可随时重新运行 init.sh。"
    exit 0
  fi

  seed_data

  print_final
}

# 5.9 部署摘要
print_summary() {
  echo ""
  separator
  printf "${BOLD}          部署摘要${NC}\n"
  separator

  printf "\n${BOLD}服务:${NC}\n"
  printf "  %-20s %s\n" "PostgreSQL" "localhost:${POSTGRES_PORT}"
  printf "  %-20s %s\n" "Syntopica" "http://localhost:${PORT}"

  if [ "$AI_MODE" = "ollama" ]; then
    printf "\n${BOLD}AI (Ollama):${NC}\n"
    printf "  %-20s %s\n" "端点" "http://${AI_IP}:${TEXT_PORT}"
    printf "  %-20s %s\n" "文本模型" "$TEXT_MODEL"
    printf "  %-20s %s\n" "嵌入模型" "$EMBED_MODEL"
  elif [ "$AI_MODE" = "llamacpp" ]; then
    printf "\n${BOLD}AI (llama.cpp):${NC}\n"
    printf "  %-20s %s\n" "文本端点" "http://${AI_IP}:${TEXT_PORT}"
    printf "  %-20s %s\n" "嵌入端点" "http://${AI_IP}:${EMBED_PORT}"
    printf "  %-20s %s\n" "模型" "loaded-model（占位符）"
  elif [ "$AI_MODE" = "remote" ]; then
    printf "\n${BOLD}AI (远程 API):${NC}\n"
    printf "  %-20s %s\n" "端点" "$AI_BASE_URL"
    printf "  %-20s %s\n" "文本模型" "$TEXT_MODEL"
    printf "  %-20s %s\n" "嵌入模型" "$EMBED_MODEL"
  else
    printf "\n${BOLD}AI:${NC} 已跳过\n"
  fi

  if [ "$FIRECRAWL_MODE" = "self" ]; then
    printf "\n${BOLD}Firecrawl:${NC} 自部署\n"
  elif [ "$FIRECRAWL_MODE" = "cloud" ]; then
    printf "\n${BOLD}Firecrawl:${NC} 云端 ($FIRECRAWL_API_URL)\n"
  else
    printf "\n${BOLD}Firecrawl:${NC} 已跳过\n"
  fi

  echo ""
  separator
}

# 5.10 种子数据写入
seed_data() {
  step "写入种子数据"

  local api_base="http://localhost:${PORT}/api"
  local code

  if [ "$AI_MODE" = "skip" ]; then
    info "AI 配置已跳过，跳过种子数据写入"
  else
    local text_api_key="$AI_API_KEY"
    if [ "$AI_PROVIDER_TYPE" = "ollama" ]; then
      text_api_key=""
    fi

    # 创建文本 Provider
    info "创建文本 AI Provider..."
    local text_provider_name="local-llm"
    if [ "$AI_MODE" = "remote" ]; then
      text_provider_name="remote-llm"
    fi

    code=$(curl_silent -X POST "$api_base/ai/providers" \
      -H "Content-Type: application/json" \
      -d "{
        \"name\": \"${text_provider_name}\",
        \"provider_type\": \"${AI_PROVIDER_TYPE}\",
        \"base_url\": \"${AI_BASE_URL}\",
        \"api_key\": \"${text_api_key}\",
        \"model\": \"${TEXT_MODEL}\",
        \"enabled\": true
      }")
    if [ "$code" = "200" ] || [ "$code" = "201" ]; then
      ok "文本 Provider 已创建: $text_provider_name ($TEXT_MODEL)"
    else
      warn "文本 Provider 创建失败 (HTTP $code)，可稍后在 Web UI 中配置"
    fi

    # 创建嵌入 Provider
    info "创建嵌入 AI Provider..."
    local embed_provider_name="local-embedding"
    if [ "$AI_MODE" = "remote" ]; then
      embed_provider_name="remote-embedding"
    fi

    code=$(curl_silent -X POST "$api_base/ai/providers" \
      -H "Content-Type: application/json" \
      -d "{
        \"name\": \"${embed_provider_name}\",
        \"provider_type\": \"${AI_PROVIDER_TYPE}\",
        \"base_url\": \"${EMBED_BASE_URL}\",
        \"api_key\": \"${text_api_key}\",
        \"model\": \"${EMBED_MODEL}\",
        \"enabled\": true
      }")
    if [ "$code" = "200" ] || [ "$code" = "201" ]; then
      ok "嵌入 Provider 已创建: $embed_provider_name ($EMBED_MODEL)"
    else
      warn "嵌入 Provider 创建失败 (HTTP $code)，可稍后在 Web UI 中配置"
    fi

    # 获取 Provider ID 并绑定路由
    info "绑定 AI 路由..."
    local providers_json
    providers_json=$(curl -sS "$api_base/ai/providers" 2>/dev/null || echo '{"data":[]}')

    # 从 data 数组中按 name 提取 id
    # 每个对象包含 "id":N,"name":"xxx"，按 name 匹配后取 id
    local text_provider_id embed_provider_id
    text_provider_id=$(echo "$providers_json" | grep -o '"id":[0-9]*,"name":"[^"]*"' | grep -E '"(local-llm|remote-llm)"' | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*' || echo "")
    embed_provider_id=$(echo "$providers_json" | grep -o '"id":[0-9]*,"name":"[^"]*"' | grep -E '"(local-embedding|remote-embedding)"' | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*' || echo "")

    if [ -n "$text_provider_id" ]; then
      for route in article_completion topic_tagging open_notebook; do
        code=$(curl_silent -X PUT "$api_base/ai/routes/$route" \
          -H "Content-Type: application/json" \
          -d "{\"name\":\"default\",\"enabled\":true,\"provider_ids\":[${text_provider_id}]}")
        [ "$code" = "200" ] && ok "路由已绑定: $route" || warn "路由绑定失败: $route"
      done
    fi

    if [ -n "$embed_provider_id" ]; then
      code=$(curl_silent -X PUT "$api_base/ai/routes/embedding" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"default\",\"enabled\":true,\"provider_ids\":[${embed_provider_id}]}")
      [ "$code" = "200" ] && ok "路由已绑定: embedding" || warn "路由绑定失败: embedding"
    fi
  fi

  # Firecrawl 配置
  if [ "$FIRECRAWL_MODE" != "skip" ]; then
    info "写入 Firecrawl 配置..."
    code=$(curl_silent -X POST "$api_base/firecrawl/settings" \
      -H "Content-Type: application/json" \
      -d "{
        \"enabled\": true,
        \"api_url\": \"${FIRECRAWL_API_URL}\",
        \"api_key\": \"${FIRECRAWL_API_KEY:-}\",
        \"mode\": \"default\",
        \"timeout\": 30,
        \"max_content_length\": 500000
      }")
    [ "$code" = "200" ] && ok "Firecrawl 配置已保存" || warn "Firecrawl 配置保存失败"
  fi
}

# 5.11 最终输出
print_final() {
  echo ""
  separator
  printf "${GREEN}${BOLD}          安装完成！${NC}\n"
  separator

  echo ""
  printf "${BOLD}访问地址:${NC}\n"
  printf "  ${CYAN}Syntopica:${NC} http://localhost:${PORT}\n"

  if [ "$AI_MODE" != "skip" ]; then
    printf "\n${BOLD}AI 连接:${NC}\n"
    if [ "$AI_MODE" = "ollama" ]; then
      printf "  ${CYAN}类型:${NC}     Ollama\n"
      printf "  ${CYAN}端点:${NC} http://${AI_IP}:${TEXT_PORT}\n"
      printf "  ${CYAN}文本:${NC}     ${TEXT_MODEL}\n"
      printf "  ${CYAN}嵌入:${NC}    ${EMBED_MODEL}\n"
    elif [ "$AI_MODE" = "llamacpp" ]; then
      printf "  ${CYAN}类型:${NC}     llama.cpp\n"
      printf "  ${CYAN}文本:${NC}     http://${AI_IP}:${TEXT_PORT}\n"
      printf "  ${CYAN}嵌入:${NC}    http://${AI_IP}:${EMBED_PORT}\n"
    elif [ "$AI_MODE" = "remote" ]; then
      printf "  ${CYAN}类型:${NC}     远程 API\n"
      printf "  ${CYAN}端点:${NC} ${AI_BASE_URL}\n"
      printf "  ${CYAN}文本:${NC}     ${TEXT_MODEL}\n"
      printf "  ${CYAN}嵌入:${NC}    ${EMBED_MODEL}\n"
    fi
  fi

  if [ "$FIRECRAWL_MODE" = "self" ]; then
    printf "\n${CYAN}Firecrawl:${NC} http://localhost:3002\n"
  fi

  echo ""
  printf "${BOLD}常用命令:${NC}\n"
  echo "  启动服务:     $DOCKER_COMPOSE up -d"
  echo "  停止服务:     $DOCKER_COMPOSE down"
  echo "  查看日志:     $DOCKER_COMPOSE logs -f"
  echo "  重新配置:     bash init.sh"
  echo ""

  if [ "$AI_MODE" = "ollama" ]; then
    printf "${YELLOW}提示:${NC} 使用前请确保 Ollama 已启动: ${BOLD}ollama serve${NC}\n"
  elif [ "$AI_MODE" = "llamacpp" ]; then
    printf "${YELLOW}提示:${NC} 使用 AI 功能前请先启动 llama.cpp 服务。\n"
  fi

  separator
  printf "${DIM}配置已保存到 .env${NC}\n"
}

# ════════════════════════════════════════════════════════════════════════════
# 主流程
# ════════════════════════════════════════════════════════════════════════════
main() {
  echo ""
  printf "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}\n"
  printf "${BOLD}${CYAN}║          Syntopica 安装向导                  ║${NC}\n"
  printf "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}\n"
  echo ""

  phase1
  phase2
  phase3
}

main "$@"
