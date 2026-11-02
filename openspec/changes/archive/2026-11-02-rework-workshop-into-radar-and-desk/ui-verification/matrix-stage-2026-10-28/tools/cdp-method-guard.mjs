#!/usr/bin/env node
/**
 * CDP 按 method 阻断闸（S9 验收安全边界）
 *
 * 目的：在浏览器协议层（Fetch domain）拒绝一切非 GET/HEAD/OPTIONS 的请求，
 * 而不是在页面里晚包装 window.fetch（ofetch 可能已捕获原生 fetch）。
 *
 * 用法：node cdp-method-guard.mjs <browser-ws-url> <jsonl-log-path> [marker]
 * 输出：JSONL，每行一个事件：
 *   {"kind":"blocked","ts":..,"method":"POST","url":..,"resourceType":"Fetch","sessionId":..}
 *   {"kind":"attach","ts":..,"targetType":"page","sessionId":..}
 *   {"kind":"ready","ts":..}
 * 只有 GET/HEAD/OPTIONS 被 continueRequest；其余一律 failRequest(BlockedByClient)。
 */
import { appendFileSync } from 'node:fs'

const [wsUrl, logPath, marker = ''] = process.argv.slice(2)
if (!wsUrl || !logPath) {
  console.error('usage: cdp-method-guard.mjs <ws-url> <log-path> [marker]')
  process.exit(1)
}

const ALLOWED = new Set(['GET', 'HEAD', 'OPTIONS'])
const enabledSessions = new Set()
let nextId = 1
const pending = new Map()
let ws
let closed = false

function log(obj) {
  try {
    appendFileSync(logPath, JSON.stringify({ marker, ts: new Date().toISOString(), ...obj }) + '\n')
  }
  catch (err) {
    console.error('log write failed', err)
  }
}

function send(method, params = {}, sessionId) {
  const id = nextId++
  const payload = { id, method, params }
  if (sessionId)
    payload.sessionId = sessionId
  ws.send(JSON.stringify(payload))
  return new Promise((resolve) => pending.set(id, resolve))
}

async function enableFetch(sessionId, targetType) {
  if (enabledSessions.has(sessionId))
    return
  enabledSessions.add(sessionId)
  const res = await send('Fetch.enable', {
    patterns: [{ urlPattern: '*', requestStage: 'Request' }],
    handleAuthRequests: false,
  }, sessionId)
  log({ kind: 'attach', targetType, sessionId, ok: !res.error, error: res.error?.message })
}

function handlePaused(params, sessionId) {
  const method = (params.request?.method || '').toUpperCase()
  if (ALLOWED.has(method)) {
    send('Fetch.continueRequest', { requestId: params.requestId }, sessionId).catch(() => {})
    return
  }
  log({
    kind: 'blocked',
    method,
    url: params.request?.url,
    resourceType: params.resourceType,
    frameId: params.frameId,
    hasPostData: !!params.request?.postData,
  })
  send('Fetch.failRequest', { requestId: params.requestId, errorReason: 'BlockedByClient' }, sessionId)
    .catch(() => {})
}

async function attachTarget(targetInfo) {
  const res = await send('Target.attachToTarget', { targetId: targetInfo.targetId, flatten: true })
  if (res.error) {
    log({ kind: 'attachError', targetType: targetInfo.type, error: res.error.message })
    return
  }
  await enableFetch(res.result.sessionId, targetInfo.type)
}

function connect() {
  ws = new WebSocket(wsUrl)
  ws.addEventListener('open', async () => {
    log({ kind: 'ready', url: wsUrl })
    await send('Target.setAutoAttach', { autoAttach: true, waitForDebuggerOnStart: false, flatten: true })
    const targets = await send('Target.getTargets')
    const list = targets.result?.targetInfos || []
    for (const t of list) {
      if (['page', 'iframe', 'worker', 'service_worker', 'shared_worker', 'webview'].includes(t.type))
        await attachTarget(t)
    }
  })

  ws.addEventListener('message', (ev) => {
    let msg
    try {
      msg = JSON.parse(typeof ev.data === 'string' ? ev.data : ev.data.toString())
    }
    catch {
      return
    }
    if (msg.id) {
      const resolve = pending.get(msg.id)
      if (resolve) {
        pending.delete(msg.id)
        resolve(msg)
      }
      return
    }
    if (msg.method === 'Target.attachedToTarget') {
      const { sessionId, targetInfo } = msg.params
      if (['page', 'iframe', 'worker', 'service_worker', 'shared_worker', 'webview'].includes(targetInfo.type))
        enableFetch(sessionId, targetInfo.type)
    }
    else if (msg.method === 'Target.detachedFromTarget') {
      enabledSessions.delete(msg.params.sessionId)
    }
    else if (msg.method === 'Fetch.requestPaused') {
      handlePaused(msg.params, msg.sessionId)
    }
  })

  ws.addEventListener('close', () => {
    if (closed)
      return
    log({ kind: 'closed' })
    // 闸断了就不能继续验收：退出码 2 让调用方发现并重启
    process.exit(2)
  })
  ws.addEventListener('error', (err) => {
    log({ kind: 'wsError', error: String(err?.message || err) })
  })
}

process.on('SIGTERM', () => {
  closed = true
  log({ kind: 'stopped' })
  process.exit(0)
})
process.on('SIGINT', () => {
  closed = true
  log({ kind: 'stopped' })
  process.exit(0)
})

connect()
