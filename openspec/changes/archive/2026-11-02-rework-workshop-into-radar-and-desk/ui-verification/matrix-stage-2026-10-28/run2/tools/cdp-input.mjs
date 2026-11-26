#!/usr/bin/env node
/**
 * 一次性 CDP 输入辅助（S9 run2）：wheel / touch / key，用于“真实用户意图”取消验证。
 * 仅向当前验证页派发输入事件，不改页面状态、不写业务。
 *
 * 用法：node cdp-input.mjs <browser-ws-url> '<json-actions>'
 * actions: [
 *   {"type":"wheel","x":700,"y":220,"dy":300,"dx":0,"repeat":2,"gap":80},
 *   {"type":"touch","x":700,"y":500,"phase":"start|move|end"},
 *   {"type":"key","key":"PageDown","code":"PageDown","keyCode":34},
 *   {"type":"mouse","x":700,"y":220,"action":"move|down|up"}
 * ]
 * 输出 JSONL 到 stdout，便于存档。
 */
const [wsUrl, actionsArg] = process.argv.slice(2)
if (!wsUrl || !actionsArg) {
  console.error('usage: cdp-input.mjs <ws-url> <json-actions>')
  process.exit(1)
}
const actions = JSON.parse(actionsArg)

let nextId = 1
const pending = new Map()
const ws = new WebSocket(wsUrl)
const sleep = ms => new Promise(r => setTimeout(r, ms))

function send(method, params = {}, sessionId) {
  const id = nextId++
  const payload = { id, method, params }
  if (sessionId)
    payload.sessionId = sessionId
  ws.send(JSON.stringify(payload))
  return new Promise((resolve) => pending.set(id, resolve))
}

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
  }
})

async function findPageSession() {
  const targets = await send('Target.getTargets')
  const pages = (targets.result?.targetInfos || []).filter(t => t.type === 'page' && /localhost:5100/.test(t.url))
  if (!pages.length)
    throw new Error('no matching page target')
  const attached = await send('Target.attachToTarget', { targetId: pages[0].targetId, flatten: true })
  return attached.result.sessionId
}

async function run() {
  const sessionId = await findPageSession()
  // 触摸模拟（仅用于 pointerType=touch 的用户意图事件；非真机）
  await send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 }, sessionId)
  for (const a of actions) {
    if (a.type === 'wheel') {
      const repeat = a.repeat ?? 1
      for (let i = 0; i < repeat; i++) {
        await send('Input.dispatchMouseEvent', {
          type: 'mouseWheel',
          x: a.x,
          y: a.y,
          deltaX: a.dx ?? 0,
          deltaY: a.dy ?? 0,
          button: 'none',
          buttons: 0,
          pointerType: 'mouse',
        }, sessionId)
        if (a.gap)
          await sleep(a.gap)
      }
      console.log(JSON.stringify({ type: 'wheel', x: a.x, y: a.y, dy: a.dy, repeat }))
    }
    else if (a.type === 'touch') {
      const touchPoints = a.phase === 'end'
        ? []
        : [{ x: a.x, y: a.y, id: 1, radiusX: 2, radiusY: 2, force: 1 }]
      await send('Input.dispatchTouchEvent', {
        type: a.phase === 'start' ? 'touchStart' : a.phase === 'move' ? 'touchMove' : 'touchEnd',
        touchPoints,
      }, sessionId)
      console.log(JSON.stringify({ type: 'touch', x: a.x, y: a.y, phase: a.phase }))
    }
    else if (a.type === 'key') {
      await send('Input.dispatchKeyEvent', {
        type: 'rawKeyDown',
        key: a.key,
        code: a.code,
        windowsVirtualKeyCode: a.keyCode,
        nativeVirtualKeyCode: a.keyCode,
      }, sessionId)
      await send('Input.dispatchKeyEvent', {
        type: 'keyUp',
        key: a.key,
        code: a.code,
        windowsVirtualKeyCode: a.keyCode,
        nativeVirtualKeyCode: a.keyCode,
      }, sessionId)
      console.log(JSON.stringify({ type: 'key', key: a.key }))
    }
    else if (a.type === 'mouse') {
      await send('Input.dispatchMouseEvent', {
        type: a.action === 'move' ? 'mouseMoved' : a.action === 'down' ? 'mousePressed' : 'mouseReleased',
        x: a.x,
        y: a.y,
        button: a.action === 'move' ? 'none' : 'left',
        buttons: a.action === 'move' ? 0 : 1,
        clickCount: 1,
        pointerType: 'mouse',
      }, sessionId)
      console.log(JSON.stringify({ type: 'mouse', x: a.x, y: a.y, action: a.action }))
    }
    if (a.wait)
      await sleep(a.wait)
  }
  await sleep(120)
  process.exit(0)
}

ws.addEventListener('open', () => {
  run().catch((err) => {
    console.error('failed', err)
    process.exit(1)
  })
})
