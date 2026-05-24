#!/usr/bin/env pwsh

# ── 颜色 ──
$RED = "`e[31m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$BLUE = "`e[34m"
$CYAN = "`e[36m"
$BOLD = "`e[1m"
$DIM = "`e[2m"
$NC = "`e[0m"

# ── 辅助函数 ─────────────────────────────────────────────────────────────────
function info  { Write-Host "${BLUE}ℹ${NC} $($args -join ' ')" }
function ok    { Write-Host "${GREEN}✔${NC} $($args -join ' ')" }
function warn  { Write-Host "${YELLOW}⚠${NC} $($args -join ' ')" }
function fail  { Write-Host "${RED}✖${NC} $($args -join ' ')"; exit 1 }
function step  { Write-Host "`n${BOLD}${CYAN}── $($args -join ' ') ──${NC}" }

function separator { Write-Host "${DIM}─────────────────────────────────────────────────────${NC}" }

function ask {
  param([string]$prompt, [string]$default = "")
  if ($default) {
    $response = Read-Host "${YELLOW}?${NC} ${prompt} [${default}]"
  } else {
    $response = Read-Host "${YELLOW}?${NC} ${prompt}"
  }
  $script:answer = if ($response) { $response } else { $default }
}

function ask_yn {
  param([string]$prompt, [string]$default = "Y")
  while ($true) {
    $response = Read-Host "${YELLOW}?${NC} ${prompt} [${default}]"
    if (-not $response) { $response = $default }
    if ($response -match '^[Yy]([Ee][Ss])?$') { return $true }
    if ($response -match '^[Nn]([Oo])?$') { return $false }
    warn "请输入 Y 或 n"
  }
}

function Get-HttpCode {
  [CmdletBinding()]
  param(
    [string]$Url,
    [string]$Method = "GET",
    [string]$Body,
    [string]$ContentType,
    [int]$TimeoutSec = 10
  )
  try {
    $params = @{
      Uri = $Url; Method = $Method
      TimeoutSec = $TimeoutSec; UseBasicParsing = $true
      ErrorAction = "Stop"
    }
    if ($Body) { $params.Body = $Body; $params.ContentType = $ContentType }
    $response = Invoke-WebRequest @params
    return $response.StatusCode
  } catch {
    if ($_.Exception.Response) { return [int]$_.Exception.Response.StatusCode }
    return 000
  }
}

function Invoke-ApiJson {
  param([string]$Url, [string]$Method = "GET", [string]$Body)
  try {
    $params = @{ Uri = $Url; Method = $Method; UseBasicParsing = $true; ErrorAction = "Stop" }
    if ($Body) { $params.Body = $Body; $params.ContentType = "application/json" }
    return Invoke-RestMethod @params
  } catch { return $null }
}

# ── 状态变量 ─────────────────────────────────────────────────────────────────
$PORT = "5000"
$POSTGRES_PORT = "5432"
$POSTGRES_DB = "syntopica"
$POSTGRES_USER = "postgres"
$POSTGRES_PASSWORD = "postgres"
$TZ = "Asia/Shanghai"

$AI_MODE = ""
$AI_IP = ""
$TEXT_PORT = ""
$EMBED_PORT = ""
$TEXT_MODEL = ""
$EMBED_MODEL = ""
$AI_PROVIDER_TYPE = ""
$AI_API_KEY = ""
$AI_BASE_URL = ""
$EMBED_BASE_URL = ""

$FIRECRAWL_MODE = ""
$FIRECRAWL_API_URL = ""
$FIRECRAWL_API_KEY = ""

$DOCKER_COMPOSE = $null
$DOCKER_COMPOSE_STR = ""
$SCRIPT_DIR = $PSScriptRoot
if (-not $SCRIPT_DIR) { $SCRIPT_DIR = (Get-Location).Path }

# ════════════════════════════════════════════════════════════════════════════
# 阶段 1：核心启动
# ════════════════════════════════════════════════════════════════════════════
function phase1 {
  step "阶段 1：核心启动"
  check_docker
  collect_config
  merge_env
  start_services
  wait_healthy
}

function check_docker {
  info "检查 Docker..."
  $dc = Get-Command "docker" -ErrorAction SilentlyContinue
  if (-not $dc) {
    fail "Docker 未安装。请先安装 Docker Desktop。`n  https://www.docker.com/products/docker-desktop/"
  }

  $composeVer = & docker compose version 2>$null
  if ($LASTEXITCODE -eq 0) {
    $script:DOCKER_COMPOSE = { docker compose @args }
    $script:DOCKER_COMPOSE_STR = "docker compose"
  } else {
    $legacy = Get-Command "docker-compose" -ErrorAction SilentlyContinue
    if ($legacy) {
      $script:DOCKER_COMPOSE = { docker-compose @args }
      $script:DOCKER_COMPOSE_STR = "docker-compose"
    } else {
      fail "docker compose 插件未找到。请更新 Docker Desktop。"
    }
  }
  ok "Docker 和 compose 可用"
}

function collect_config {
  step "配置"
  Write-Host "按 Enter 接受默认值。`n"

  ask "服务端口" $PORT; $script:PORT = $script:answer
  ask "PostgreSQL 端口" $POSTGRES_PORT; $script:POSTGRES_PORT = $script:answer
  ask "PostgreSQL 数据库名" $POSTGRES_DB; $script:POSTGRES_DB = $script:answer
  ask "PostgreSQL 用户名" $POSTGRES_USER; $script:POSTGRES_USER = $script:answer
  ask "PostgreSQL 密码" $POSTGRES_PASSWORD; $script:POSTGRES_PASSWORD = $script:answer
  ask "时区" $TZ; $script:TZ = $script:answer
}

function merge_env {
  step "生成 .env"

  $env_file = "$SCRIPT_DIR\.env"
  $example_file = "$SCRIPT_DIR\.env.example"

  if (-not (Test-Path $example_file)) {
    warn ".env.example 未找到，创建最小 .env"
    @"
PORT=$PORT
POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_PORT=$POSTGRES_PORT
TZ=$TZ
"@ | Set-Content $env_file
    ok ".env 已创建"
    return
  }

  $template = Get-Content $example_file

  $existing = @{}
  if (Test-Path $env_file) {
    Get-Content $env_file | ForEach-Object {
      if ($_ -match '^([^=]+)=(.*)') { $existing[$matches[1]] = $matches[2] }
    }
  }

  $defaults = @{
    PORT = $script:PORT
    POSTGRES_DB = $script:POSTGRES_DB
    POSTGRES_USER = $script:POSTGRES_USER
    POSTGRES_PASSWORD = $script:POSTGRES_PASSWORD
    POSTGRES_PORT = $script:POSTGRES_PORT
    TZ = $script:TZ
  }

  $result = $template | ForEach-Object {
    $line = $_
    if ($line -match '^([^=]+)=(.*)') {
      $key = $matches[1]
      if ($existing.ContainsKey($key) -and -not [string]::IsNullOrEmpty($existing[$key])) {
        "$key=$($existing[$key])"
      } elseif ($defaults.ContainsKey($key)) {
        "$key=$($defaults[$key])"
      } else {
        $line
      }
    } else {
      $line
    }
  }

  $result | Set-Content $env_file

  foreach ($key in $defaults.Keys) {
    if ($existing.ContainsKey($key)) {
      Set-Variable -Name $key -Value $existing[$key] -Scope Script -ErrorAction SilentlyContinue
    }
  }

  ok ".env 已更新（已有值已保留）"
}

function start_services {
  step "启动服务"
  info "运行 docker compose up -d..."
  & $script:DOCKER_COMPOSE -f docker-compose.yml up -d 2>&1 | Out-Null
  if ($LASTEXITCODE -ne 0) { fail "Docker 服务启动失败" }
  ok "Docker 服务已启动"
}

function wait_healthy {
  step "等待服务就绪"

  info "等待 PostgreSQL..."
  $retries = 30
  while ($retries -gt 0) {
    $null = & $script:DOCKER_COMPOSE exec -T postgres pg_isready -U $POSTGRES_USER -d $POSTGRES_DB 2>$null
    if ($LASTEXITCODE -eq 0) { break }
    $retries--
    Start-Sleep -Seconds 2
  }
  if ($retries -eq 0) { fail "PostgreSQL 未能在规定时间内就绪" }
  ok "PostgreSQL 已就绪"

  info "等待后端 /health..."
  $retries = 60
  $code = "000"
  while ($retries -gt 0) {
    $code = Get-HttpCode -Url "http://localhost:${PORT}/health"
    if ($code -eq 200) { break }
    $retries--
    Start-Sleep -Seconds 3
  }
  if ($retries -eq 0) { fail "后端 /health 未返回 200（最后状态码: $code）" }
  ok "后端服务健康"
}

# ════════════════════════════════════════════════════════════════════════════
# 阶段 2：可选配置
# ════════════════════════════════════════════════════════════════════════════
function phase2 {
  step "阶段 2：可选配置"

  detect_ip
  ask_ai_mode
  if ($AI_MODE -ne "skip") {
    health_check_ai
  }

  ask_firecrawl_mode
}

# 5.3 IP 自动检测
function detect_ip {
  $ipOutput = ipconfig 2>$null
  $script:AI_IP = ($ipOutput | Select-String -Pattern '(\d+\.\d+\.\d+\.\d+)' | Select-Object -First 1).Matches.Groups[1].Value

  if (-not $script:AI_IP) {
    $script:AI_IP = "localhost"
    warn "无法自动检测本机 IP，使用 localhost（Docker 内可能无法访问）"
  } else {
    info "检测到本机 IP: $AI_IP"
  }
}

# 5.2 AI 模式选择
function ask_ai_mode {
  step "AI 模型配置"
  Write-Host "如何连接 AI 模型？"
  Write-Host ""
  Write-Host "  1) Ollama"
  Write-Host "  2) llama.cpp（本地服务）"
  Write-Host "  3) 远程 API（OpenAI 兼容）"
  Write-Host "  4) 暂时跳过"
  Write-Host ""

  while ($true) {
    ask "请选择 [1-4]" "4"
    switch ($script:answer) {
      "1" { $script:AI_MODE = "ollama"; setup_ollama; break }
      "2" { $script:AI_MODE = "llamacpp"; setup_llamacpp; break }
      "3" { $script:AI_MODE = "remote"; setup_remote; break }
      "4" { $script:AI_MODE = "skip"; info "已跳过 AI 配置"; break }
      default { warn "请输入 1、2、3 或 4" }
    }
    if (@("ollama", "llamacpp", "remote", "skip") -contains $script:AI_MODE) { break }
  }
}

# 5.4 Ollama 配置
function setup_ollama {
  $script:AI_PROVIDER_TYPE = "ollama"
  $script:TEXT_PORT = "11434"
  $script:EMBED_PORT = "11434"

  ask "AI 服务 IP 地址" $AI_IP; $script:AI_IP = $script:answer
  ask "Ollama 端口" $TEXT_PORT; $script:TEXT_PORT = $script:answer
  $script:EMBED_PORT = $script:TEXT_PORT

  $script:AI_BASE_URL = "http://${AI_IP}:${TEXT_PORT}/v1"
  $script:EMBED_BASE_URL = $script:AI_BASE_URL

  ask "文本模型名称" "qwen3:8b"; $script:TEXT_MODEL = $script:answer
  ask "嵌入模型名称" "nomic-embed-text"; $script:EMBED_MODEL = $script:answer
  $script:AI_API_KEY = ""
}

# 5.5 llama.cpp 配置
function setup_llamacpp {
  $script:AI_PROVIDER_TYPE = "openai_compatible"
  $script:TEXT_PORT = "8080"
  $script:EMBED_PORT = "8081"

  ask "AI 服务 IP 地址" $AI_IP; $script:AI_IP = $script:answer
  ask "文本服务端口" $TEXT_PORT; $script:TEXT_PORT = $script:answer
  ask "嵌入服务端口" $EMBED_PORT; $script:EMBED_PORT = $script:answer

  $script:AI_BASE_URL = "http://${AI_IP}:${TEXT_PORT}/v1"
  $script:EMBED_BASE_URL = "http://${AI_IP}:${EMBED_PORT}/v1"

  $script:TEXT_MODEL = "loaded-model"
  $script:EMBED_MODEL = "loaded-model"
  $script:AI_API_KEY = "sk-local"
}

# 5.6 远程 API 配置
function setup_remote {
  $script:AI_PROVIDER_TYPE = "openai_compatible"

  ask "API 地址" "https://api.openai.com/v1"; $script:AI_BASE_URL = $script:answer
  $script:EMBED_BASE_URL = $script:AI_BASE_URL
  ask "API 密钥"; $script:AI_API_KEY = $script:answer
  ask "文本模型名称" "gpt-4o-mini"; $script:TEXT_MODEL = $script:answer
  ask "嵌入模型名称" "text-embedding-3-small"; $script:EMBED_MODEL = $script:answer
}

# 5.7 AI 健康检测
function health_check_ai {
  step "AI 健康检查"

  if ($AI_MODE -eq "remote") {
    info "远程 API 模式，跳过健康检查"
    return
  }

  $healthUrl = "http://${AI_IP}:${TEXT_PORT}/v1/chat/completions"
  info "测试 AI 服务 ${AI_IP}:${TEXT_PORT}..."

  $body = @{
    model = $TEXT_MODEL
    messages = @(@{ role = "user"; content = "hello" })
    max_tokens = 5
  } | ConvertTo-Json -Compress

  $retries = 3
  while ($retries -gt 0) {
    $code = Get-HttpCode -Url $healthUrl -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
    if ($code -eq 200) {
      ok "AI 服务连接正常"
      return
    }
    $retries--
    if ($retries -gt 0) {
      warn "AI 服务未响应 (HTTP $code)，${retries} 次重试剩余..."
      Start-Sleep -Seconds 20
    }
  }

  warn "无法连接 AI 服务: $healthUrl"
  if (-not (ask_yn "是否继续？（可稍后在 Web UI 中配置）" "Y")) {
    $script:AI_MODE = "skip"
    info "已跳过 AI 配置，种子数据不会写入"
  }
}

# 5.8 Firecrawl 模式
function ask_firecrawl_mode {
  step "Firecrawl 配置"
  Write-Host "如何处理全文爬取？"
  Write-Host ""
  Write-Host "  1) 自部署 Firecrawl (docker-compose.firecrawl.yml)"
  Write-Host "  2) 云端 API（使用 Firecrawl 云服务）"
  Write-Host "  3) 跳过 Firecrawl 配置"
  Write-Host ""

  while ($true) {
    ask "请选择 [1-3]" "3"
    switch ($script:answer) {
      "1" { $script:FIRECRAWL_MODE = "self"; setup_firecrawl_self; break }
      "2" { $script:FIRECRAWL_MODE = "cloud"; setup_firecrawl_cloud; break }
      "3" { $script:FIRECRAWL_MODE = "skip"; info "已跳过 Firecrawl 配置"; break }
      default { warn "请输入 1、2 或 3" }
    }
    if (@("self", "cloud", "skip") -contains $script:FIRECRAWL_MODE) { break }
  }
}

function setup_firecrawl_self {
  $yml = "$SCRIPT_DIR\docker-compose.firecrawl.yml"
  if (Test-Path $yml) {
    info "启动 Firecrawl 服务..."
    $output = & $script:DOCKER_COMPOSE -f $yml up -d 2>&1
    if ($LASTEXITCODE -ne 0) {
      warn "Firecrawl 启动失败:"
      Write-Host $output
      warn "可手动启动: docker compose -f docker-compose.firecrawl.yml up -d"
    } else {
      ok "Firecrawl 已启动"
    }
    $script:FIRECRAWL_API_URL = "http://firecrawl:3002"
  } else {
    warn "docker-compose.firecrawl.yml 未找到"
    warn "请先创建该文件，参见 docs/reference/deployment.md"
    $script:FIRECRAWL_API_URL = "http://firecrawl:3002"
  }
}

function setup_firecrawl_cloud {
  ask "Firecrawl 云端 API 地址" "https://api.firecrawl.dev/v1"; $script:FIRECRAWL_API_URL = $script:answer
  ask "Firecrawl API 密钥"; $script:FIRECRAWL_API_KEY = $script:answer
  ok "Firecrawl 云端已配置"
}

# ════════════════════════════════════════════════════════════════════════════
# 阶段 3：确认与执行
# ════════════════════════════════════════════════════════════════════════════
function phase3 {
  step "阶段 3：确认"

  print_summary

  if (-not (ask_yn "确认执行？" "Y")) {
    info "已取消。可随时重新运行 init.ps1。"
    exit 0
  }

  seed_data
  print_final
}

# 5.9 部署摘要
function print_summary {
  Write-Host ""
  separator
  Write-Host "${BOLD}          部署摘要${NC}"
  separator

  Write-Host "`n${BOLD}服务:${NC}"
  Write-Host ("  {0,-20} {1}" -f "PostgreSQL", "localhost:${POSTGRES_PORT}")
  Write-Host ("  {0,-20} {1}" -f "Syntopica", "http://localhost:${PORT}")

  if ($AI_MODE -eq "ollama") {
    Write-Host "`n${BOLD}AI (Ollama):${NC}"
    Write-Host ("  {0,-20} {1}" -f "端点", "http://${AI_IP}:${TEXT_PORT}")
    Write-Host ("  {0,-20} {1}" -f "文本模型", $TEXT_MODEL)
    Write-Host ("  {0,-20} {1}" -f "嵌入模型", $EMBED_MODEL)
  } elseif ($AI_MODE -eq "llamacpp") {
    Write-Host "`n${BOLD}AI (llama.cpp):${NC}"
    Write-Host ("  {0,-20} {1}" -f "文本端点", "http://${AI_IP}:${TEXT_PORT}")
    Write-Host ("  {0,-20} {1}" -f "嵌入端点", "http://${AI_IP}:${EMBED_PORT}")
    Write-Host ("  {0,-20} {1}" -f "模型", "loaded-model（占位符）")
  } elseif ($AI_MODE -eq "remote") {
    Write-Host "`n${BOLD}AI (远程 API):${NC}"
    Write-Host ("  {0,-20} {1}" -f "端点", $AI_BASE_URL)
    Write-Host ("  {0,-20} {1}" -f "文本模型", $TEXT_MODEL)
    Write-Host ("  {0,-20} {1}" -f "嵌入模型", $EMBED_MODEL)
  } else {
    Write-Host "`n${BOLD}AI:${NC} 已跳过"
  }

  if ($FIRECRAWL_MODE -eq "self") {
    Write-Host "`n${BOLD}Firecrawl:${NC} 自部署"
  } elseif ($FIRECRAWL_MODE -eq "cloud") {
    Write-Host "`n${BOLD}Firecrawl:${NC} 云端 ($FIRECRAWL_API_URL)"
  } else {
    Write-Host "`n${BOLD}Firecrawl:${NC} 已跳过"
  }

  Write-Host ""
  separator
}

# 5.10 种子数据写入
function seed_data {
  step "写入种子数据"

  $apiBase = "http://localhost:${PORT}/api"

  if ($AI_MODE -eq "skip") {
    info "AI 配置已跳过，跳过种子数据写入"
  } else {
    $apiKey = $AI_API_KEY
    if ($AI_PROVIDER_TYPE -eq "ollama") { $apiKey = "" }

    # 创建文本 Provider
    info "创建文本 AI Provider..."
    $textProviderName = if ($AI_MODE -eq "remote") { "remote-llm" } else { "local-llm" }

    $textBody = @{
      name = $textProviderName
      provider_type = $AI_PROVIDER_TYPE
      base_url = $AI_BASE_URL
      api_key = $apiKey
      model = $TEXT_MODEL
      enabled = $true
    } | ConvertTo-Json -Compress

    $code = Get-HttpCode -Url "$apiBase/ai/providers" -Method Post -Body $textBody -ContentType "application/json" -TimeoutSec 15
    if ($code -eq 200 -or $code -eq 201) {
      ok "文本 Provider 已创建: $textProviderName ($TEXT_MODEL)"
    } else {
      warn "文本 Provider 创建失败 (HTTP $code)，可稍后在 Web UI 中配置"
    }

    # 创建嵌入 Provider
    info "创建嵌入 AI Provider..."
    $embedProviderName = if ($AI_MODE -eq "remote") { "remote-embedding" } else { "local-embedding" }

    $embedBody = @{
      name = $embedProviderName
      provider_type = $AI_PROVIDER_TYPE
      base_url = $EMBED_BASE_URL
      api_key = $apiKey
      model = $EMBED_MODEL
      enabled = $true
    } | ConvertTo-Json -Compress

    $code = Get-HttpCode -Url "$apiBase/ai/providers" -Method Post -Body $embedBody -ContentType "application/json" -TimeoutSec 15
    if ($code -eq 200 -or $code -eq 201) {
      ok "嵌入 Provider 已创建: $embedProviderName ($EMBED_MODEL)"
    } else {
      warn "嵌入 Provider 创建失败 (HTTP $code)，可稍后在 Web UI 中配置"
    }

    # 获取 Provider ID 并绑定路由
    info "绑定 AI 路由..."
    $resp = Invoke-ApiJson -Url "$apiBase/ai/providers"
    $providerList = if ($resp -and $resp.data) { $resp.data } else { @() }

    $textProviderId = ($providerList | Where-Object { $_.name -in @("local-llm", "remote-llm") } | Select-Object -First 1).id
    $embedProviderId = ($providerList | Where-Object { $_.name -in @("local-embedding", "remote-embedding") } | Select-Object -First 1).id

    if ($textProviderId) {
      foreach ($route in @("article_completion", "topic_tagging", "open_notebook")) {
        $routeBody = @{ name = "default"; enabled = $true; provider_ids = @($textProviderId) } | ConvertTo-Json -Compress
        $code = Get-HttpCode -Url "$apiBase/ai/routes/$route" -Method Put -Body $routeBody -ContentType "application/json" -TimeoutSec 15
        if ($code -eq 200 -or $code -eq 201) { ok "路由已绑定: $route" } else { warn "路由绑定失败: $route" }
      }
    }

    if ($embedProviderId) {
      $routeBody = @{ name = "default"; enabled = $true; provider_ids = @($embedProviderId) } | ConvertTo-Json -Compress
      $code = Get-HttpCode -Url "$apiBase/ai/routes/embedding" -Method Put -Body $routeBody -ContentType "application/json" -TimeoutSec 15
      if ($code -eq 200 -or $code -eq 201) { ok "路由已绑定: embedding" } else { warn "路由绑定失败: embedding" }
    }
  }

  # Firecrawl 配置
  if ($FIRECRAWL_MODE -ne "skip") {
    info "写入 Firecrawl 配置..."
    $fcBody = @{
      enabled = $true
      api_url = $FIRECRAWL_API_URL
      api_key = if ($FIRECRAWL_API_KEY) { $FIRECRAWL_API_KEY } else { "" }
      mode = "default"
      timeout = 30
      max_content_length = 500000
    } | ConvertTo-Json -Compress

    $code = Get-HttpCode -Url "$apiBase/firecrawl/settings" -Method Post -Body $fcBody -ContentType "application/json" -TimeoutSec 15
    if ($code -eq 200 -or $code -eq 201) { ok "Firecrawl 配置已保存" } else { warn "Firecrawl 配置保存失败" }
  }
}

# 5.11 最终输出
function print_final {
  Write-Host ""
  separator
  Write-Host "${GREEN}${BOLD}          安装完成！${NC}"
  separator

  Write-Host "`n${BOLD}访问地址:${NC}"
  Write-Host "  ${CYAN}Syntopica:${NC} http://localhost:${PORT}"

  if ($AI_MODE -ne "skip") {
    Write-Host "`n${BOLD}AI 连接:${NC}"
    if ($AI_MODE -eq "ollama") {
      Write-Host "  ${CYAN}类型:${NC}     Ollama"
      Write-Host "  ${CYAN}端点:${NC} http://${AI_IP}:${TEXT_PORT}"
      Write-Host "  ${CYAN}文本:${NC}     ${TEXT_MODEL}"
      Write-Host "  ${CYAN}嵌入:${NC}    ${EMBED_MODEL}"
    } elseif ($AI_MODE -eq "llamacpp") {
      Write-Host "  ${CYAN}类型:${NC}     llama.cpp"
      Write-Host "  ${CYAN}文本:${NC}     http://${AI_IP}:${TEXT_PORT}"
      Write-Host "  ${CYAN}嵌入:${NC}    http://${AI_IP}:${EMBED_PORT}"
    } elseif ($AI_MODE -eq "remote") {
      Write-Host "  ${CYAN}类型:${NC}     远程 API"
      Write-Host "  ${CYAN}端点:${NC} ${AI_BASE_URL}"
      Write-Host "  ${CYAN}文本:${NC}     ${TEXT_MODEL}"
      Write-Host "  ${CYAN}嵌入:${NC}    ${EMBED_MODEL}"
    }
  }

  if ($FIRECRAWL_MODE -eq "self") {
    Write-Host "`n${CYAN}Firecrawl:${NC} http://localhost:3002"
  }

  Write-Host "`n${BOLD}常用命令:${NC}"
  Write-Host "  启动服务:     $($script:DOCKER_COMPOSE_STR) up -d"
  Write-Host "  停止服务:     $($script:DOCKER_COMPOSE_STR) down"
  Write-Host "  查看日志:     $($script:DOCKER_COMPOSE_STR) logs -f"
  Write-Host "  重新配置:     .\init.ps1"
  Write-Host ""

  if ($AI_MODE -eq "ollama") {
    Write-Host "${YELLOW}提示:${NC} 使用前请确保 Ollama 已启动: ${BOLD}ollama serve${NC}"
  } elseif ($AI_MODE -eq "llamacpp") {
    Write-Host "${YELLOW}提示:${NC} 使用 AI 功能前请先启动 llama.cpp 服务。"
  }

  separator
  Write-Host "${DIM}配置已保存到 .env${NC}"
}

# ════════════════════════════════════════════════════════════════════════════
# 主流程
# ════════════════════════════════════════════════════════════════════════════
Write-Host ""
Write-Host "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}"
Write-Host "${BOLD}${CYAN}║          Syntopica 安装向导                  ║${NC}"
Write-Host "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}"
Write-Host ""

phase1
phase2
phase3
