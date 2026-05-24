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
      *) warn "Please enter Y or n" ;;
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
# PHASE 1: Core Startup
# ════════════════════════════════════════════════════════════════════════════
phase1() {
  step "Phase 1: Core Startup"

  check_docker

  collect_config

  merge_env

  start_services

  wait_healthy
}

check_docker() {
  info "Checking Docker..."
  if ! command -v docker &>/dev/null; then
    fail "Docker is not installed. Please install Docker Desktop first.\n  https://www.docker.com/products/docker-desktop/"
  fi

  if ! docker compose version &>/dev/null 2>&1; then
    if ! docker-compose version &>/dev/null 2>&1; then
      fail "docker compose plugin not found. Please update Docker Desktop or install docker-compose."
    fi
    DOCKER_COMPOSE="docker-compose"
  else
    DOCKER_COMPOSE="docker compose"
  fi
  ok "Docker and $DOCKER_COMPOSE available"
}

collect_config() {
  step "Configuration"
  echo "Press Enter to accept defaults.\n"

  ask "Port" "$PORT"; PORT="$answer"
  ask "PostgreSQL port" "$POSTGRES_PORT"; POSTGRES_PORT="$answer"
  ask "PostgreSQL database" "$POSTGRES_DB"; POSTGRES_DB="$answer"
  ask "PostgreSQL user" "$POSTGRES_USER"; POSTGRES_USER="$answer"
  ask "PostgreSQL password" "$POSTGRES_PASSWORD"; POSTGRES_PASSWORD="$answer"
  ask "Timezone" "$TZ"; TZ="$answer"
}

merge_env() {
  step "Generating .env"

  local env_file="$SCRIPT_DIR/.env"
  local example_file="$SCRIPT_DIR/.env.example"

  if [ ! -f "$example_file" ]; then
    warn ".env.example not found, creating minimal .env"
    cat > "$env_file" <<EOF
PORT=$PORT
POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_PORT=$POSTGRES_PORT
TZ=$TZ
EOF
    ok ".env created"
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
  ok ".env updated (existing values preserved)"
}

start_services() {
  step "Starting services"

  info "Running $DOCKER_COMPOSE up -d..."
  $DOCKER_COMPOSE up -d 2>&1 || fail "Failed to start Docker services"

  ok "Docker services started"
}

wait_healthy() {
  step "Waiting for services to become healthy"

  # Wait for postgres
  info "Waiting for PostgreSQL..."
  local retries=30
  while [ $retries -gt 0 ]; do
    if $DOCKER_COMPOSE exec -T postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" &>/dev/null; then
      break
    fi
    retries=$((retries - 1))
    sleep 2
  done
  if [ $retries -eq 0 ]; then
    fail "PostgreSQL did not become healthy in time"
  fi
  ok "PostgreSQL is ready"

  # Wait for backend health
  info "Waiting for backend /health..."
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
    fail "Backend /health did not return 200 in time (last HTTP status: $code)"
  fi
  ok "Backend is healthy"
}

# ════════════════════════════════════════════════════════════════════════════
# PHASE 2: Optional Setup
# ════════════════════════════════════════════════════════════════════════════
phase2() {
  step "Phase 2: Optional Setup"

  detect_ip
  ask_ai_mode
  if [ "$AI_MODE" != "skip" ]; then
    health_check_ai
  fi

  ask_firecrawl_mode
}

# 5.3 IP auto-detection
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
    warn "Could not auto-detect host IP, using localhost (may not work from Docker)"
  else
    info "Detected host IP: $AI_IP"
  fi
}

# 5.2 AI mode selection
ask_ai_mode() {
  step "AI Model Setup"
  echo "How do you want to connect to AI models?"
  echo ""
  echo "  1) Ollama"
  echo "  2) llama.cpp (local server)"
  echo "  3) Remote API (OpenAI-compatible)"
  echo "  4) Skip AI setup for now"
  echo ""

  local choice
  while true; do
    ask "Choose [1-4]" "4"
    choice="$answer"
    case "$choice" in
      1) AI_MODE="ollama"; setup_ollama; break ;;
      2) AI_MODE="llamacpp"; setup_llamacpp; break ;;
      3) AI_MODE="remote"; setup_remote; break ;;
      4) AI_MODE="skip"; info "AI setup skipped"; break ;;
      *) warn "Enter 1, 2, 3, or 4" ;;
    esac
  done
}

# 5.4 Ollama configuration
setup_ollama() {
  AI_PROVIDER_TYPE="ollama"
  TEXT_PORT="11434"
  EMBED_PORT="11434"

  ask "AI service IP address" "$AI_IP"; AI_IP="$answer"
  ask "Ollama port" "$TEXT_PORT"; TEXT_PORT="$answer"
  EMBED_PORT="$TEXT_PORT"

  AI_BASE_URL="http://${AI_IP}:${TEXT_PORT}/v1"
  EMBED_BASE_URL="$AI_BASE_URL"

  ask "Text model name" "qwen3:8b"; TEXT_MODEL="$answer"
  ask "Embed model name" "nomic-embed-text"; EMBED_MODEL="$answer"
  AI_API_KEY=""
}

# 5.5 llama.cpp configuration
setup_llamacpp() {
  AI_PROVIDER_TYPE="openai_compatible"
  TEXT_PORT="8080"
  EMBED_PORT="8081"

  ask "AI service IP address" "$AI_IP"; AI_IP="$answer"
  ask "Text server port" "$TEXT_PORT"; TEXT_PORT="$answer"
  ask "Embed server port" "$EMBED_PORT"; EMBED_PORT="$answer"

  AI_BASE_URL="http://${AI_IP}:${TEXT_PORT}/v1"
  EMBED_BASE_URL="http://${AI_IP}:${EMBED_PORT}/v1"

  TEXT_MODEL="loaded-model"
  EMBED_MODEL="loaded-model"
  AI_API_KEY="sk-local"
}

# 5.6 Remote API configuration
setup_remote() {
  AI_PROVIDER_TYPE="openai_compatible"

  ask "API base URL" "https://api.openai.com/v1"; AI_BASE_URL="$answer"
  EMBED_BASE_URL="$AI_BASE_URL"
  ask "API key"; AI_API_KEY="$answer"
  ask "Text model name" "gpt-4o-mini"; TEXT_MODEL="$answer"
  ask "Embed model name" "text-embedding-3-small"; EMBED_MODEL="$answer"
}

# 5.7 AI health detection
health_check_ai() {
  step "AI Health Check"

  if [ "$AI_MODE" = "remote" ]; then
    info "Remote API selected, skipping health check"
    return
  fi

  local health_url="http://${AI_IP}:${TEXT_PORT}/v1/chat/completions"
  info "Testing AI service at ${AI_IP}:${TEXT_PORT}..."

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
      ok "AI service is reachable and responding"
      return
    fi

    retries=$((retries - 1))
    if [ $retries -gt 0 ]; then
      warn "AI service not responding (HTTP $code). Retrying in 5s... ($retries left)"
      sleep 5
    fi
  done

  warn "Could not reach AI service at $health_url"
  if ! ask_yn "Continue anyway? (you can configure AI later in Web UI)" "Y"; then
    AI_MODE="skip"
    info "AI setup will be skipped, seed data not written"
  fi
}

# 5.8 Firecrawl mode
ask_firecrawl_mode() {
  step "Firecrawl Setup"
  echo "How do you want to handle full-text crawling?"
  echo ""
  echo "  1) Self-deploy Firecrawl (docker-compose.firecrawl.yml)"
  echo "  2) Cloud API (use Firecrawl cloud service)"
  echo "  3) Skip Firecrawl setup"
  echo ""

  local choice
  while true; do
    ask "Choose [1-3]" "3"
    choice="$answer"
    case "$choice" in
      1) FIRECRAWL_MODE="self"; setup_firecrawl_self; break ;;
      2) FIRECRAWL_MODE="cloud"; setup_firecrawl_cloud; break ;;
      3) FIRECRAWL_MODE="skip"; info "Firecrawl setup skipped"; break ;;
      *) warn "Enter 1, 2, or 3" ;;
    esac
  done
}

setup_firecrawl_self() {
  if [ -f "$SCRIPT_DIR/docker-compose.firecrawl.yml" ]; then
    info "Starting Firecrawl services..."
    if $DOCKER_COMPOSE -f docker-compose.firecrawl.yml up -d 2>&1; then
      ok "Firecrawl started"
    else
      warn "Firecrawl startup failed"
      warn "You can start it manually: docker compose -f docker-compose.firecrawl.yml up -d"
    fi
    FIRECRAWL_API_URL="http://firecrawl:3002"
  else
    warn "docker-compose.firecrawl.yml not found"
    warn "You may need to create it first. See docs/reference/deployment.md"
    FIRECRAWL_API_URL="http://firecrawl:3002"
  fi
}

setup_firecrawl_cloud() {
  ask "Firecrawl cloud API URL" "https://api.firecrawl.dev/v1"; FIRECRAWL_API_URL="$answer"
  ask "Firecrawl API key"; FIRECRAWL_API_KEY="$answer"
  ok "Firecrawl cloud configured"
}

# ════════════════════════════════════════════════════════════════════════════
# PHASE 3: Confirmation & Execution
# ════════════════════════════════════════════════════════════════════════════
phase3() {
  step "Phase 3: Confirmation"

  print_summary

  if ! ask_yn "Proceed with setup?" "Y"; then
    info "Setup cancelled. You can re-run init.sh anytime."
    exit 0
  fi

  seed_data

  print_final
}

# 5.9 Print deployment summary
print_summary() {
  echo ""
  separator
  printf "${BOLD}          Deployment Summary${NC}\n"
  separator

  printf "\n${BOLD}Services:${NC}\n"
  printf "  %-20s %s\n" "PostgreSQL" "localhost:${POSTGRES_PORT}"
  printf "  %-20s %s\n" "Syntopica" "http://localhost:${PORT}"

  if [ "$AI_MODE" = "ollama" ]; then
    printf "\n${BOLD}AI (Ollama):${NC}\n"
    printf "  %-20s %s\n" "Endpoint" "http://${AI_IP}:${TEXT_PORT}"
    printf "  %-20s %s\n" "Text model" "$TEXT_MODEL"
    printf "  %-20s %s\n" "Embed model" "$EMBED_MODEL"
  elif [ "$AI_MODE" = "llamacpp" ]; then
    printf "\n${BOLD}AI (llama.cpp):${NC}\n"
    printf "  %-20s %s\n" "Text endpoint" "http://${AI_IP}:${TEXT_PORT}"
    printf "  %-20s %s\n" "Embed endpoint" "http://${AI_IP}:${EMBED_PORT}"
    printf "  %-20s %s\n" "Model" "loaded-model (placeholder)"
  elif [ "$AI_MODE" = "remote" ]; then
    printf "\n${BOLD}AI (Remote):${NC}\n"
    printf "  %-20s %s\n" "Endpoint" "$AI_BASE_URL"
    printf "  %-20s %s\n" "Text model" "$TEXT_MODEL"
    printf "  %-20s %s\n" "Embed model" "$EMBED_MODEL"
  else
    printf "\n${BOLD}AI:${NC} skipped\n"
  fi

  if [ "$FIRECRAWL_MODE" = "self" ]; then
    printf "\n${BOLD}Firecrawl:${NC} self-hosted\n"
  elif [ "$FIRECRAWL_MODE" = "cloud" ]; then
    printf "\n${BOLD}Firecrawl:${NC} cloud ($FIRECRAWL_API_URL)\n"
  else
    printf "\n${BOLD}Firecrawl:${NC} skipped\n"
  fi

  echo ""
  separator
}

# 5.10 Seed data write
seed_data() {
  step "Writing Seed Data"

  local api_base="http://localhost:${PORT}/api"
  local code

  if [ "$AI_MODE" = "skip" ]; then
    info "AI setup was skipped, skipping seed data"
  else
    local text_api_key="$AI_API_KEY"
    if [ "$AI_PROVIDER_TYPE" = "ollama" ]; then
      text_api_key=""
    fi

    # Create text provider
    info "Creating AI provider for text tasks..."
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
      ok "Text provider created: $text_provider_name ($TEXT_MODEL)"
    else
      warn "Failed to create text provider (HTTP $code). You can configure it in the web UI."
    fi

    # Create embed provider
    info "Creating AI provider for embedding..."
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
      ok "Embed provider created: $embed_provider_name ($EMBED_MODEL)"
    else
      warn "Failed to create embed provider (HTTP $code). You can configure it in the web UI."
    fi

    # Get provider IDs and bind routes
    info "Binding AI routes..."
    local providers_json
    providers_json=$(curl -sS "$api_base/ai/providers" 2>/dev/null || echo "[]")

    local text_provider_id embed_provider_id
    text_provider_id=$(echo "$providers_json" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2 || echo "")
    embed_provider_id=$(echo "$providers_json" | grep -o '"id":[0-9]*' | tail -1 | cut -d':' -f2 || echo "")

    if [ -n "$text_provider_id" ]; then
      for route in article_completion topic_tagging open_notebook; do
        code=$(curl_silent -X PUT "$api_base/ai/routes/$route" \
          -H "Content-Type: application/json" \
          -d "{\"name\":\"default\",\"enabled\":true,\"provider_ids\":[${text_provider_id}]}")
        [ "$code" = "200" ] && ok "Route bound: $route" || warn "Failed to bind $route route"
      done
    fi

    if [ -n "$embed_provider_id" ]; then
      code=$(curl_silent -X PUT "$api_base/ai/routes/embedding" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"default\",\"enabled\":true,\"provider_ids\":[${embed_provider_id}]}")
      [ "$code" = "200" ] && ok "Route bound: embedding" || warn "Failed to bind embedding route"
    fi
  fi

  # Firecrawl config
  if [ "$FIRECRAWL_MODE" != "skip" ]; then
    info "Writing Firecrawl settings..."
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
    [ "$code" = "200" ] && ok "Firecrawl settings saved" || warn "Failed to save Firecrawl settings"
  fi
}

# 5.11 Final output
print_final() {
  echo ""
  separator
  printf "${GREEN}${BOLD}          Setup Complete!${NC}\n"
  separator

  echo ""
  printf "${BOLD}Access URLs:${NC}\n"
  printf "  ${CYAN}Syntopica:${NC} http://localhost:${PORT}\n"

  if [ "$AI_MODE" != "skip" ]; then
    printf "\n${BOLD}AI Connection:${NC}\n"
    if [ "$AI_MODE" = "ollama" ]; then
      printf "  ${CYAN}Type:${NC}     Ollama\n"
      printf "  ${CYAN}Endpoint:${NC} http://${AI_IP}:${TEXT_PORT}\n"
      printf "  ${CYAN}Text:${NC}     ${TEXT_MODEL}\n"
      printf "  ${CYAN}Embed:${NC}    ${EMBED_MODEL}\n"
    elif [ "$AI_MODE" = "llamacpp" ]; then
      printf "  ${CYAN}Type:${NC}     llama.cpp\n"
      printf "  ${CYAN}Text:${NC}     http://${AI_IP}:${TEXT_PORT}\n"
      printf "  ${CYAN}Embed:${NC}    http://${AI_IP}:${EMBED_PORT}\n"
    elif [ "$AI_MODE" = "remote" ]; then
      printf "  ${CYAN}Type:${NC}     Remote API\n"
      printf "  ${CYAN}Endpoint:${NC} ${AI_BASE_URL}\n"
      printf "  ${CYAN}Text:${NC}     ${TEXT_MODEL}\n"
      printf "  ${CYAN}Embed:${NC}    ${EMBED_MODEL}\n"
    fi
  fi

  if [ "$FIRECRAWL_MODE" = "self" ]; then
    printf "\n${CYAN}Firecrawl:${NC} http://localhost:3002\n"
  fi

  echo ""
  printf "${BOLD}Quick Reference:${NC}\n"
  echo "  Start services:     $DOCKER_COMPOSE up -d"
  echo "  Stop services:      $DOCKER_COMPOSE down"
  echo "  View logs:          $DOCKER_COMPOSE logs -f"
  echo "  Re-run setup:       bash init.sh"
  echo ""

  if [ "$AI_MODE" = "ollama" ]; then
    printf "${YELLOW}Note:${NC} Ensure Ollama is running with: ${BOLD}ollama serve${NC}\n"
  elif [ "$AI_MODE" = "llamacpp" ]; then
    printf "${YELLOW}Note:${NC} Start llama.cpp servers ${BOLD}before${NC} using AI features.\n"
  fi

  separator
  printf "${DIM}Configuration saved to .env${NC}\n"
}

# ════════════════════════════════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════════════════════════════════
main() {
  echo ""
  printf "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}\n"
  printf "${BOLD}${CYAN}║          Syntopica Setup Wizard              ║${NC}\n"
  printf "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}\n"
  echo ""

  phase1
  phase2
  phase3
}

main "$@"
