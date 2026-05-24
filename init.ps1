#!/usr/bin/env pwsh

# ── Colors ──
$RED = "`e[31m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$BLUE = "`e[34m"
$CYAN = "`e[36m"
$BOLD = "`e[1m"
$DIM = "`e[2m"
$NC = "`e[0m"

# ── Helpers ─────────────────────────────────────────────────────────────────
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
    warn "Please enter Y or n"
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

# ── State variables ─────────────────────────────────────────────────────────
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
# PHASE 1: Core Startup
# ════════════════════════════════════════════════════════════════════════════
function phase1 {
  step "Phase 1: Core Startup"
  check_docker
  collect_config
  merge_env
  start_services
  wait_healthy
}

function check_docker {
  info "Checking Docker..."
  $dc = Get-Command "docker" -ErrorAction SilentlyContinue
  if (-not $dc) {
    fail "Docker is not installed. Please install Docker Desktop first.`n  https://www.docker.com/products/docker-desktop/"
  }

  # Check docker compose plugin
  $composeVer = & docker compose version 2>$null
  if ($LASTEXITCODE -eq 0) {
    $script:DOCKER_COMPOSE = { docker compose @args }
    $script:DOCKER_COMPOSE_STR = "docker compose"
  } else {
    # Fallback to docker-compose legacy
    $legacy = Get-Command "docker-compose" -ErrorAction SilentlyContinue
    if ($legacy) {
      $script:DOCKER_COMPOSE = { docker-compose @args }
      $script:DOCKER_COMPOSE_STR = "docker-compose"
    } else {
      fail "docker compose plugin not found. Please update Docker Desktop."
    }
  }
  ok "Docker and compose available"
}

function collect_config {
  step "Configuration"
  Write-Host "Press Enter to accept defaults.`n"

  ask "Port" $PORT; $script:PORT = $script:answer
  ask "PostgreSQL port" $POSTGRES_PORT; $script:POSTGRES_PORT = $script:answer
  ask "PostgreSQL database" $POSTGRES_DB; $script:POSTGRES_DB = $script:answer
  ask "PostgreSQL user" $POSTGRES_USER; $script:POSTGRES_USER = $script:answer
  ask "PostgreSQL password" $POSTGRES_PASSWORD; $script:POSTGRES_PASSWORD = $script:answer
  ask "Timezone" $TZ; $script:TZ = $script:answer
}

function merge_env {
  step "Generating .env"

  $env_file = "$SCRIPT_DIR\.env"
  $example_file = "$SCRIPT_DIR\.env.example"

  if (-not (Test-Path $example_file)) {
    warn ".env.example not found, creating minimal .env"
    @"
PORT=$PORT
POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_PORT=$POSTGRES_PORT
TZ=$TZ
"@ | Set-Content $env_file
    ok ".env created"
    return
  }

  # Read template
  $template = Get-Content $example_file

  # Read existing .env
  $existing = @{}
  if (Test-Path $env_file) {
    Get-Content $env_file | ForEach-Object {
      if ($_ -match '^([^=]+)=(.*)') { $existing[$matches[1]] = $matches[2] }
    }
  }

  # Defaults from script variables
  $defaults = @{
    PORT = $script:PORT
    POSTGRES_DB = $script:POSTGRES_DB
    POSTGRES_USER = $script:POSTGRES_USER
    POSTGRES_PASSWORD = $script:POSTGRES_PASSWORD
    POSTGRES_PORT = $script:POSTGRES_PORT
    TZ = $script:TZ
  }

  # Process template: preserve existing values, fill defaults, keep template
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

  # Update script variables from existing .env values
  foreach ($key in $defaults.Keys) {
    if ($existing.ContainsKey($key)) {
      Set-Variable -Name $key -Value $existing[$key] -Scope Script -ErrorAction SilentlyContinue
    }
  }

  ok ".env updated (existing values preserved)"
}

function start_services {
  step "Starting services"
  info "Running docker compose up -d for PostgreSQL..."
  & $script:DOCKER_COMPOSE -f docker-compose.yml up -d 2>&1 | Out-Null
  if ($LASTEXITCODE -ne 0) { fail "Failed to start Docker services" }
  ok "Docker services started"
}

function wait_healthy {
  step "Waiting for services to become healthy"

  # Wait for postgres
  info "Waiting for PostgreSQL..."
  $retries = 30
  while ($retries -gt 0) {
    $null = & $script:DOCKER_COMPOSE exec -T postgres pg_isready -U $POSTGRES_USER -d $POSTGRES_DB 2>$null
    if ($LASTEXITCODE -eq 0) { break }
    $retries--
    Start-Sleep -Seconds 2
  }
  if ($retries -eq 0) { fail "PostgreSQL did not become healthy in time" }
  ok "PostgreSQL is ready"

  # Wait for backend health
  info "Waiting for backend /health..."
  $retries = 60
  $code = "000"
  while ($retries -gt 0) {
    $code = Get-HttpCode -Url "http://localhost:${PORT}/health"
    if ($code -eq 200) { break }
    $retries--
    Start-Sleep -Seconds 3
  }
  if ($retries -eq 0) { fail "Backend /health did not return 200 in time (last HTTP status: $code)" }
  ok "Backend is healthy"
}

# ════════════════════════════════════════════════════════════════════════════
# PHASE 2: Optional Setup
# ════════════════════════════════════════════════════════════════════════════
function phase2 {
  step "Phase 2: Optional Setup"

  detect_ip
  ask_ai_mode
  if ($AI_MODE -ne "skip") {
    health_check_ai
  }

  ask_firecrawl_mode
}

# 5.3 IP auto-detection
function detect_ip {
  # Windows: parse ipconfig for first IPv4 address
  $ipOutput = ipconfig 2>$null
  $script:AI_IP = ($ipOutput | Select-String -Pattern '(\d+\.\d+\.\d+\.\d+)' | Select-Object -First 1).Matches.Groups[1].Value

  if (-not $script:AI_IP) {
    $script:AI_IP = "localhost"
    warn "Could not auto-detect host IP, using localhost (may not work from Docker)"
  } else {
    info "Detected host IP: $AI_IP"
  }
}

# 5.2 AI mode selection
function ask_ai_mode {
  step "AI Model Setup"
  Write-Host "How do you want to connect to AI models?"
  Write-Host ""
  Write-Host "  1) Ollama"
  Write-Host "  2) llama.cpp (local server)"
  Write-Host "  3) Remote API (OpenAI-compatible)"
  Write-Host "  4) Skip AI setup for now"
  Write-Host ""

  while ($true) {
    ask "Choose [1-4]" "4"
    switch ($script:answer) {
      "1" { $script:AI_MODE = "ollama"; setup_ollama; break }
      "2" { $script:AI_MODE = "llamacpp"; setup_llamacpp; break }
      "3" { $script:AI_MODE = "remote"; setup_remote; break }
      "4" { $script:AI_MODE = "skip"; info "AI setup skipped"; break }
      default { warn "Enter 1, 2, 3, or 4" }
    }
    if (@("ollama", "llamacpp", "remote", "skip") -contains $script:AI_MODE) { break }
  }
}

# 5.4 Ollama configuration
function setup_ollama {
  $script:AI_PROVIDER_TYPE = "ollama"
  $script:TEXT_PORT = "11434"
  $script:EMBED_PORT = "11434"

  ask "AI service IP address" $AI_IP; $script:AI_IP = $script:answer
  ask "Ollama port" $TEXT_PORT; $script:TEXT_PORT = $script:answer
  $script:EMBED_PORT = $script:TEXT_PORT

  $script:AI_BASE_URL = "http://${AI_IP}:${TEXT_PORT}/v1"
  $script:EMBED_BASE_URL = $script:AI_BASE_URL

  ask "Text model name" "qwen3:8b"; $script:TEXT_MODEL = $script:answer
  ask "Embed model name" "nomic-embed-text"; $script:EMBED_MODEL = $script:answer
  $script:AI_API_KEY = ""
}

# 5.5 llama.cpp configuration
function setup_llamacpp {
  $script:AI_PROVIDER_TYPE = "openai_compatible"
  $script:TEXT_PORT = "8080"
  $script:EMBED_PORT = "8081"

  ask "AI service IP address" $AI_IP; $script:AI_IP = $script:answer
  ask "Text server port" $TEXT_PORT; $script:TEXT_PORT = $script:answer
  ask "Embed server port" $EMBED_PORT; $script:EMBED_PORT = $script:answer

  $script:AI_BASE_URL = "http://${AI_IP}:${TEXT_PORT}/v1"
  $script:EMBED_BASE_URL = "http://${AI_IP}:${EMBED_PORT}/v1"

  $script:TEXT_MODEL = "loaded-model"
  $script:EMBED_MODEL = "loaded-model"
  $script:AI_API_KEY = "sk-local"
}

# 5.6 Remote API configuration
function setup_remote {
  $script:AI_PROVIDER_TYPE = "openai_compatible"

  ask "API base URL" "https://api.openai.com/v1"; $script:AI_BASE_URL = $script:answer
  $script:EMBED_BASE_URL = $script:AI_BASE_URL
  ask "API key"; $script:AI_API_KEY = $script:answer
  ask "Text model name" "gpt-4o-mini"; $script:TEXT_MODEL = $script:answer
  ask "Embed model name" "text-embedding-3-small"; $script:EMBED_MODEL = $script:answer
}

# 5.7 AI health detection
function health_check_ai {
  step "AI Health Check"

  if ($AI_MODE -eq "remote") {
    info "Remote API selected, skipping health check"
    return
  }

  $healthUrl = "http://${AI_IP}:${TEXT_PORT}/v1/chat/completions"
  info "Testing AI service at ${AI_IP}:${TEXT_PORT}..."

  $body = @{
    model = $TEXT_MODEL
    messages = @(@{ role = "user"; content = "hello" })
    max_tokens = 5
  } | ConvertTo-Json -Compress

  $retries = 3
  while ($retries -gt 0) {
    $code = Get-HttpCode -Url $healthUrl -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
    if ($code -eq 200) {
      ok "AI service is reachable and responding"
      return
    }
    $retries--
    if ($retries -gt 0) {
      warn "AI service not responding (HTTP $code). Retrying in 20s... ($retries left)"
      Start-Sleep -Seconds 20
    }
  }

  warn "Could not reach AI service at $healthUrl"
  if (-not (ask_yn "Continue anyway? (you can configure AI later in Web UI)" "Y")) {
    $script:AI_MODE = "skip"
    info "AI setup will be skipped, seed data not written"
  }
}

# 5.8 Firecrawl mode
function ask_firecrawl_mode {
  step "Firecrawl Setup"
  Write-Host "How do you want to handle full-text crawling?"
  Write-Host ""
  Write-Host "  1) Self-deploy Firecrawl (docker-compose.firecrawl.yml)"
  Write-Host "  2) Cloud API (use Firecrawl cloud service)"
  Write-Host "  3) Skip Firecrawl setup"
  Write-Host ""

  while ($true) {
    ask "Choose [1-3]" "3"
    switch ($script:answer) {
      "1" { $script:FIRECRAWL_MODE = "self"; setup_firecrawl_self; break }
      "2" { $script:FIRECRAWL_MODE = "cloud"; setup_firecrawl_cloud; break }
      "3" { $script:FIRECRAWL_MODE = "skip"; info "Firecrawl setup skipped"; break }
      default { warn "Enter 1, 2, or 3" }
    }
    if (@("self", "cloud", "skip") -contains $script:FIRECRAWL_MODE) { break }
  }
}

function setup_firecrawl_self {
  $yml = "$SCRIPT_DIR\docker-compose.firecrawl.yml"
  if (Test-Path $yml) {
    info "Starting Firecrawl services..."
    $output = & $script:DOCKER_COMPOSE -f $yml up -d 2>&1
    if ($LASTEXITCODE -ne 0) {
      warn "Firecrawl startup failed:"
      Write-Host $output
      warn "You can start it manually: docker compose -f docker-compose.firecrawl.yml up -d"
    } else {
      ok "Firecrawl started"
    }
    $script:FIRECRAWL_API_URL = "http://firecrawl:3002"
  } else {
    warn "docker-compose.firecrawl.yml not found"
    warn "You may need to create it first. See docs/reference/deployment.md"
    $script:FIRECRAWL_API_URL = "http://firecrawl:3002"
  }
}

function setup_firecrawl_cloud {
  ask "Firecrawl cloud API URL" "https://api.firecrawl.dev/v1"; $script:FIRECRAWL_API_URL = $script:answer
  ask "Firecrawl API key"; $script:FIRECRAWL_API_KEY = $script:answer
  ok "Firecrawl cloud configured"
}

# ════════════════════════════════════════════════════════════════════════════
# PHASE 3: Confirmation & Execution
# ════════════════════════════════════════════════════════════════════════════
function phase3 {
  step "Phase 3: Confirmation"

  print_summary

  if (-not (ask_yn "Proceed with setup?" "Y")) {
    info "Setup cancelled. You can re-run init.ps1 anytime."
    exit 0
  }

  seed_data
  print_final
}

# 5.9 Print deployment summary
function print_summary {
  Write-Host ""
  separator
  Write-Host "${BOLD}          Deployment Summary${NC}"
  separator

  Write-Host "`n${BOLD}Services:${NC}"
  Write-Host ("  {0,-20} {1}" -f "PostgreSQL", "localhost:${POSTGRES_PORT}")
  Write-Host ("  {0,-20} {1}" -f "Syntopica", "http://localhost:${PORT}")

  if ($AI_MODE -eq "ollama") {
    Write-Host "`n${BOLD}AI (Ollama):${NC}"
    Write-Host ("  {0,-20} {1}" -f "Endpoint", "http://${AI_IP}:${TEXT_PORT}")
    Write-Host ("  {0,-20} {1}" -f "Text model", $TEXT_MODEL)
    Write-Host ("  {0,-20} {1}" -f "Embed model", $EMBED_MODEL)
  } elseif ($AI_MODE -eq "llamacpp") {
    Write-Host "`n${BOLD}AI (llama.cpp):${NC}"
    Write-Host ("  {0,-20} {1}" -f "Text endpoint", "http://${AI_IP}:${TEXT_PORT}")
    Write-Host ("  {0,-20} {1}" -f "Embed endpoint", "http://${AI_IP}:${EMBED_PORT}")
    Write-Host ("  {0,-20} {1}" -f "Model", "loaded-model (placeholder)")
  } elseif ($AI_MODE -eq "remote") {
    Write-Host "`n${BOLD}AI (Remote):${NC}"
    Write-Host ("  {0,-20} {1}" -f "Endpoint", $AI_BASE_URL)
    Write-Host ("  {0,-20} {1}" -f "Text model", $TEXT_MODEL)
    Write-Host ("  {0,-20} {1}" -f "Embed model", $EMBED_MODEL)
  } else {
    Write-Host "`n${BOLD}AI:${NC} skipped"
  }

  if ($FIRECRAWL_MODE -eq "self") {
    Write-Host "`n${BOLD}Firecrawl:${NC} self-hosted"
  } elseif ($FIRECRAWL_MODE -eq "cloud") {
    Write-Host "`n${BOLD}Firecrawl:${NC} cloud ($FIRECRAWL_API_URL)"
  } else {
    Write-Host "`n${BOLD}Firecrawl:${NC} skipped"
  }

  Write-Host ""
  separator
}

# 5.10 Seed data write
function seed_data {
  step "Writing Seed Data"

  $apiBase = "http://localhost:${PORT}/api"

  if ($AI_MODE -eq "skip") {
    info "AI setup was skipped, skipping seed data"
  } else {
    $apiKey = $AI_API_KEY
    if ($AI_PROVIDER_TYPE -eq "ollama") { $apiKey = "" }

    # Create text provider
    info "Creating AI provider for text tasks..."
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
      ok "Text provider created: $textProviderName ($TEXT_MODEL)"
    } else {
      warn "Failed to create text provider (HTTP $code). You can configure it in the web UI."
    }

    # Create embed provider
    info "Creating AI provider for embedding..."
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
      ok "Embed provider created: $embedProviderName ($EMBED_MODEL)"
    } else {
      warn "Failed to create embed provider (HTTP $code). You can configure it in the web UI."
    }

    # Get provider IDs and bind routes
    info "Binding AI routes..."
    $providers = Invoke-ApiJson -Url "$apiBase/ai/providers"
    $textProviderId = if ($providers -and $providers[0]) { $providers[0].id } else { $null }
    $embedProviderId = if ($providers -and $providers[1]) { $providers[1].id } else { $null }

    if ($textProviderId) {
      foreach ($route in @("article_completion", "topic_tagging", "open_notebook")) {
        $routeBody = @{ name = "default"; enabled = $true; provider_ids = @($textProviderId) } | ConvertTo-Json -Compress
        $code = Get-HttpCode -Url "$apiBase/ai/routes/$route" -Method Put -Body $routeBody -ContentType "application/json" -TimeoutSec 15
        if ($code -eq 200 -or $code -eq 201) { ok "Route bound: $route" } else { warn "Failed to bind $route route" }
      }
    }

    if ($embedProviderId) {
      $routeBody = @{ name = "default"; enabled = $true; provider_ids = @($embedProviderId) } | ConvertTo-Json -Compress
      $code = Get-HttpCode -Url "$apiBase/ai/routes/embedding" -Method Put -Body $routeBody -ContentType "application/json" -TimeoutSec 15
      if ($code -eq 200 -or $code -eq 201) { ok "Route bound: embedding" } else { warn "Failed to bind embedding route" }
    }
  }

  # Firecrawl config
  if ($FIRECRAWL_MODE -ne "skip") {
    info "Writing Firecrawl settings..."
    $fcBody = @{
      enabled = $true
      api_url = $FIRECRAWL_API_URL
      api_key = if ($FIRECRAWL_API_KEY) { $FIRECRAWL_API_KEY } else { "" }
      mode = "default"
      timeout = 30
      max_content_length = 500000
    } | ConvertTo-Json -Compress

    $code = Get-HttpCode -Url "$apiBase/firecrawl/settings" -Method Post -Body $fcBody -ContentType "application/json" -TimeoutSec 15
    if ($code -eq 200 -or $code -eq 201) { ok "Firecrawl settings saved" } else { warn "Failed to save Firecrawl settings" }
  }
}

# 5.11 Final output
function print_final {
  Write-Host ""
  separator
  Write-Host "${GREEN}${BOLD}          Setup Complete!${NC}"
  separator

  Write-Host "`n${BOLD}Access URLs:${NC}"
  Write-Host "  ${CYAN}Syntopica:${NC} http://localhost:${PORT}"

  if ($AI_MODE -ne "skip") {
    Write-Host "`n${BOLD}AI Connection:${NC}"
    if ($AI_MODE -eq "ollama") {
      Write-Host "  ${CYAN}Type:${NC}     Ollama"
      Write-Host "  ${CYAN}Endpoint:${NC} http://${AI_IP}:${TEXT_PORT}"
      Write-Host "  ${CYAN}Text:${NC}     ${TEXT_MODEL}"
      Write-Host "  ${CYAN}Embed:${NC}    ${EMBED_MODEL}"
    } elseif ($AI_MODE -eq "llamacpp") {
      Write-Host "  ${CYAN}Type:${NC}     llama.cpp"
      Write-Host "  ${CYAN}Text:${NC}     http://${AI_IP}:${TEXT_PORT}"
      Write-Host "  ${CYAN}Embed:${NC}    http://${AI_IP}:${EMBED_PORT}"
    } elseif ($AI_MODE -eq "remote") {
      Write-Host "  ${CYAN}Type:${NC}     Remote API"
      Write-Host "  ${CYAN}Endpoint:${NC} ${AI_BASE_URL}"
      Write-Host "  ${CYAN}Text:${NC}     ${TEXT_MODEL}"
      Write-Host "  ${CYAN}Embed:${NC}    ${EMBED_MODEL}"
    }
  }

  if ($FIRECRAWL_MODE -eq "self") {
    Write-Host "`n${CYAN}Firecrawl:${NC} http://localhost:3002"
  }

  Write-Host "`n${BOLD}Quick Reference:${NC}"
  Write-Host "  Start services:     $($script:DOCKER_COMPOSE_STR) up -d"
  Write-Host "  Stop services:      $($script:DOCKER_COMPOSE_STR) down"
  Write-Host "  View logs:          $($script:DOCKER_COMPOSE_STR) logs -f"
  Write-Host "  Re-run setup:       .\init.ps1"
  Write-Host ""

  if ($AI_MODE -eq "ollama") {
    Write-Host "${YELLOW}Note:${NC} Ensure Ollama is running with: ${BOLD}ollama serve${NC}"
  } elseif ($AI_MODE -eq "llamacpp") {
    Write-Host "${YELLOW}Note:${NC} Start llama.cpp servers ${BOLD}before${NC} using AI features."
  }

  separator
  Write-Host "${DIM}Configuration saved to .env${NC}"
}

# ════════════════════════════════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════════════════════════════════
Write-Host ""
Write-Host "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}"
Write-Host "${BOLD}${CYAN}║          Syntopica Setup Wizard              ║${NC}"
Write-Host "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}"
Write-Host ""

phase1
phase2
phase3
