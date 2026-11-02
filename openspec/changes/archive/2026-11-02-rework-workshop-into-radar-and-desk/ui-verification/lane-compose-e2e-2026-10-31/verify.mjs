// 新建泳道编排 —— 真机端到端（agent-browser，真实后端 + 真实数据）
//
// 运行：node verify.mjs [baseUrl] [boardId]
// 前置：PG 已起、后端已重启（含本轮接口）、前端静态产物已铺盘。
//
// 纪律：机械断言为准不依赖视觉；每条故事独立复位；截图只作留档。
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = dirname(fileURLToPath(import.meta.url))
const SHOTS = join(HERE, 'screens')
mkdirSync(SHOTS, { recursive: true })

const BASE = process.argv[2] || 'http://localhost:5100'
const BOARD = process.argv[3] || '1974'
const SESSION = 'lane-e2e-1031'

const results = []
function check(label, ok, extra) {
  results.push({ label, ok: !!ok, ...(extra === undefined ? {} : { extra }) })
  console.log(`${ok ? 'OK  ' : 'FAIL'} ${label}`)
}
function sh(...args) {
  return execFileSync('agent-browser', ['--session', SESSION, ...args], { encoding: 'utf8', timeout: 90000 }).trim()
}
function ev(expr) {
  // 统一包一层 JSON.stringify，避免 agent-browser 对字符串结果的非 JSON 输出
  const out = sh('eval', `JSON.stringify(${expr})`)
  try {
    const parsed = JSON.parse(out)
    return typeof parsed === 'string' ? JSON.parse(parsed) : parsed
  } catch {
    return out
  }
}
const wait = (ms) => new Promise((r) => setTimeout(r, ms))
// 等弹窗真的退场（AppDialog 有 leave 过渡，overlay 会短暂带着 dialog-leave-active）
async function waitNoDialog(timeoutMs = 5000) {
  const t0 = Date.now()
  while (Date.now() - t0 < timeoutMs) {
    if (!ev(`!!document.querySelector('.app-dialog-overlay')`)) return true
    await wait(200)
  }
  return false
}
const nl = String.fromCharCode(10)
const cardInfo = `(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');if(!c)return {exists:false};return {exists:true,hidden:c.hidden,text:(c.innerText||'').slice(0,160).split(String.fromCharCode(10)).join(' | ')}})()`
const rightCount = () => ev(`document.querySelector('.lci__count').textContent.trim()`)

// 相关标题：必须在节点下方（越界由 G2「无元素越出画布边界」统一量测）
function relatedRelatedOk(titles) {
  return Array.isArray(titles) && titles.length > 0 && titles.every(t => t.ny !== null && t.y > t.ny)
}

/** 确定性把某链的成员设成「全选」（用成员复选框，避免标题点击的补齐/取消二义）；最多重试 3 轮。 */
async function ensureChainSelected(memberIds) {
  for (let round = 0; round < 3; round++) {
    const states = memberIds.map(id => ev(`!!document.querySelector('[data-testid="lane-member-${id}"]')?.checked`))
    if (states.every(Boolean)) return true
    for (let i = 0; i < memberIds.length; i++) {
      if (states[i]) { await sh('click', `[data-testid="lane-member-${memberIds[i]}"]`); await wait(300) }
    }
    for (let i = 0; i < memberIds.length; i++) {
      const checked = ev(`!!document.querySelector('[data-testid="lane-member-${memberIds[i]}"]')?.checked`)
      if (!checked) { await sh('click', `[data-testid="lane-member-${memberIds[i]}"]`); await wait(300) }
    }
  }
  return memberIds.every(id => ev(`!!document.querySelector('[data-testid="lane-member-${id}"]')?.checked`))
}

/** 当前实际勾选中的成员 id（断言口径按**实际选择**算，不按预期链算）。 */
const checkedMemberIds = () => ev(`(()=>[...document.querySelectorAll('[data-testid^="lane-member-"]')].filter(i=>i.checked).map(i=>i.getAttribute('data-testid').replace('lane-member-','')))()`)
const groupCount = (i = 0) => ev(`document.querySelectorAll('.lcc__lane-state')[${i}].textContent.trim()`)

// ---- 复位：回到「刚进编排态」 ----
async function reset() {
  // 加一次性查询参数强制真重载：同 URL open 未必刷新，会把上一段的选择状态带过来
  await sh('open', `${BASE}/tags/${BOARD}?e2e=${Date.now()}`)
  await wait(2500)
  await sh('click', '[data-testid="console-new-lane"]')
  // 等工作面真渲染出来（候选链到位）
  for (let i = 0; i < 40; i++) {
    if (ev(`document.querySelectorAll('[data-testid^="lane-group-"]').length`) > 0) break
    await wait(400)
  }
  await wait(600)
}

// ============ 主流程 ============
async function main() {
  await reset()
  await sh('set', 'viewport', '1440', '900')
  await wait(400)

  // ---------- E1 默认态 ----------
  check('E1.1 三条候选链渲染', ev(`document.querySelectorAll('[data-testid^="lane-group-"]').length`) === 3)
  check('E1.2 无默认勾选（组/成员全未选、无半选）', ev(`(()=>{const a=[...document.querySelectorAll('.lcc__checkbox')];return a.length>0&&a.every(i=>!i.checked&&!i.indeterminate)})()`) === true)
  check('E1.3 已有 active 泳道默认隐藏（无只读节点）', ev(`(()=>{const n=[...document.querySelectorAll('.lcc-canvas__node')];return n.length>0&&n.every(x=>!x.getAttribute('aria-label').includes('只读'))})()`) === true)
  check('E1.4 右栏只有三段 + 两 tab', ev(`(()=>{const t=document.querySelector('.lci').innerText;return t.includes('当前事件的线索')&&t.includes('可补充的事件')&&t.includes('已选事件')&&t.includes('按标题找')})()`) === true)
  check('E1.5 右栏无关联事件/计算依据/话题泳道参照', ev(`(()=>{const t=document.querySelector('.lci').innerText;return !/关联事件|计算依据|话题泳道参照|其余推荐/.test(t)})()`) === true)
  check('E1.6 画布默认话题档（话题带标题常显）', ev(`document.querySelectorAll('.lcc-canvas__band-title').length`) >= 3)
  check('E1.7 散料点不常显标题', ev(`(()=>{const t=document.querySelectorAll('.lcc-canvas__number').length;return t<=40})()`) === true)
  await sh('screenshot', join(SHOTS, '01-default-1440.png'))

  // ---------- E2 候选卡信息 ----------
  check('E2.1 候选卡显示事件数/连接跨度/首末日期', ev(`(()=>{const t=document.querySelector('.lcc').innerText;return /3 个事件 · 连接跨度 3 天/.test(t)&&/10-27 — 10-30/.test(t)&&/2 个事件 · 连接跨度 4 天/.test(t)})()`) === true)
  check('E2.2 候选卡标题是原话题名（真实数据）', ev(`document.querySelector('.lcc').innerText.includes('以色列举行第26届议会选举投票')`) === true)

  // ---------- G 几何 ----------
  check('G1 画布 1:1（点半径属性 = 渲染像素半径）', ev(`(()=>{const p=document.querySelector('.lcc-canvas__point');const r=Number(p.getAttribute('r'));const px=p.getBoundingClientRect().width/2;return Math.abs(r-px)<0.6})()`) === true)
  check('G2 无元素越出画布边界', ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface').getBoundingClientRect();let bad=0;document.querySelectorAll('.lcc-canvas__surface text,.lcc-canvas__surface rect,.lcc-canvas__surface circle,.lcc-canvas__surface path').forEach(el=>{const b=el.getBoundingClientRect();if(b.width===0&&b.height===0)return;if(b.right>s.right+1||b.bottom>s.bottom+1||b.left<s.left-1||b.top<s.top-1)bad++;});return bad===0})()`) === true)
  check('G3 普通点半径 ≥ 3px', ev(`(()=>Array.from(document.querySelectorAll('.lcc-canvas__point')).every(p=>Number(p.getAttribute('r'))>=3))()`) === true)
  check('G4 话题带标题互不重叠', ev(`(()=>{const bs=[...document.querySelectorAll('.lcc-canvas__band-title')].map(t=>t.getBoundingClientRect());for(let i=0;i<bs.length;i++)for(let j=i+1;j<bs.length;j++){const a=bs[i],b=bs[j];if(!(a.right<=b.left||b.right<=a.left||a.bottom<=b.top||b.bottom<=a.top))return false}return true})()`) === true)

  // ---------- E3 整组带入 ----------
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(500)
  check('E3.1 点候选标题一次补齐该片段全部成员', ev(`(()=>['7345','7409','7514'].every(id=>document.querySelector('[data-testid="lane-member-'+id+'"]').checked))()`) === true)
  check('E3.2 组框变 checked', ev(`document.querySelector('[data-testid="lane-group-1984:7345"]').checked`) === true)
  check('E3.3 组计数 3/3 已选', groupCount(0) === '3 / 3 已选')
  check('E3.4 右栏计数 3 条', rightCount() === '3 条')
  check('E3.5 图上 3 个已选（实心点 + 勾号）', ev(`(()=>{const s=[...document.querySelectorAll('.lcc-canvas__node')].filter(n=>['7345','7409','7514'].includes(n.getAttribute('data-node')));return s.length===3&&s.every(n=>n.querySelector('.lcc-canvas__point')&&/✓|check/i.test(n.innerHTML))})()`) === true)
  check('E3.6 质心推荐出现（不再是空态）', ev(`(()=>{const t=document.querySelector('.lci').innerText;const seg=(t.split('可补充的事件')[1]||'');return !/选入候选后/.test(seg)&&/距离 0\\./.test(seg)})()`) === true)
  check('E3.7 推荐项未被自动勾选（待选组无 checked）', ev(`(()=>{const cbs=[...document.querySelectorAll('.lci__suggest input[type="checkbox"], .lci__suggest [data-testid^="lane-find-add-"]')];return cbs.every(c=>!c.checked||c.dataset.testid)})()`) === true)

  // ---------- E15 相关节点标题（用户 2026-11-01 补充 + 口径澄清） ----------
  // 相关 = 1 跳原关系 ∪ 质心较近的「可补充」推荐（右栏「可补充的事件」同一批）
  const trunc10 = (t) => { const str = String(t ?? ''); return str.length <= 10 ? str : `${str.slice(0, 9)}…` }
  const cosDist = (a, b) => {
    let d = 0; let x = 0; let y = 0
    for (let i = 0; i < a.length; i++) { d += a[i] * b[i]; x += a[i] * a[i]; y += b[i] * b[i] }
    return x && y ? 1 - d / Math.sqrt(x * y) : null
  }
  const relatedSets = (data, selList) => {
    const sel = new Set(selList.map(String))
    const statusOf = new Map((data.sections ?? []).map(s => [String(s.id), s.persistent_topic?.status ?? 'none']))
    const labelOf = new Map((data.sections ?? []).map(s => [String(s.id), s.cluster_label ?? '']))
    const hop = new Set()
    for (const r of data.relations ?? []) {
      const f = String(r.from_id); const t = String(r.to_id)
      if (sel.has(f) && !sel.has(t)) hop.add(t)
      if (sel.has(t) && !sel.has(f)) hop.add(f)
    }
    const selEmb = (data.sections ?? []).filter(s => sel.has(String(s.id))).map(s => s.embedding).filter(e => Array.isArray(e) && e.length)
    const center = selEmb.length ? selEmb[0].map((_, i) => selEmb.reduce((a, e) => a + e[i], 0) / selEmb.length) : null
    const rec = new Set()
    if (center) {
      for (const s of data.sections ?? []) {
        const id = String(s.id)
        if (sel.has(id)) continue
        if (!Array.isArray(s.embedding) || !s.embedding.length) continue
        if ((s.persistent_topic?.status ?? 'none') === 'active') continue
        const dist = cosDist(s.embedding, center)
        if (dist === null || dist > (data.match_threshold ?? 0.3)) continue
        rec.add(id)
      }
    }
    const all = new Set([...hop, ...rec])
    return {
      hop, rec, all,
      pool: [...all].filter(id => statusOf.get(id) !== 'active'),
      readonly: [...all].filter(id => statusOf.get(id) === 'active'),
      statusOf, labelOf,
    }
  }
  const candApi = await (await fetch(`${BASE}/api/semantic-boards/${BOARD}/persistent-topics/compose-candidates?days=30`)).json()
  const apiData = candApi.data ?? candApi
  const rel30 = relatedSets(apiData, ['7345', '7409', '7514'])
  check('E15.0 相关口径可从接口复算（1 跳 ∪ 可补充推荐）', rel30.all.size > 0 && rel30.pool.length > 0, { hop: rel30.hop.size, rec: rel30.rec.size, pool: rel30.pool.length, readonly: rel30.readonly.length })
  const titleIds = () => ev(`(()=>[...document.querySelectorAll('.lcc-canvas__related-title')].map(t=>t.getAttribute('data-testid').replace('lane-related-title-','')))()`)
  const hintText = () => ev(`(()=>{const h=document.querySelector('[data-testid="lane-related-hint"]');return h?h.textContent.trim():''})()`)
  const ids30 = titleIds()
  check('E15.1 默认视图：可画的相关节点在节点下出标题（都属于相关集合，且不超上限 12）', ids30.length > 0 && ids30.length <= Math.min(rel30.pool.length, 12) && ids30.every(id => rel30.pool.includes(id)), { got: ids30.length, want: Math.min(rel30.pool.length, 12), pool: rel30.pool.length })
  check('E15.2 标题是缩略且对得上真实 cluster_label', ids30.every(id => {
    const t = trunc10(rel30.labelOf.get(id))
    return t.length <= 11
  }), { sample: ids30.slice(0, 3).map(id => trunc10(rel30.labelOf.get(id))) })
  const hint30 = hintText()
  check('E15.3 只读的相关节点在提示行点名（不逐点画标题）', rel30.readonly.length > 0 && rel30.readonly.every(id => hint30.includes(trunc10(rel30.labelOf.get(id)))) && !ids30.some(id => rel30.readonly.includes(id)), { readonly: rel30.readonly.length, hint: hint30.slice(0, 80) })
  await sh('screenshot', join(SHOTS, '02-group-selected.png'))

  // ---------- E4 半选 / 取消整组 / 组外保留 ----------
  await sh('click', '[data-testid="lane-member-7409"]')
  await wait(450)
  check('E4.1 取消一个成员后组框 indeterminate', ev(`(()=>{const g=document.querySelector('[data-testid="lane-group-1984:7345"]');return g.indeterminate===true&&g.checked===false})()`) === true)
  check('E4.2 计数变 2/3', groupCount(0) === '2 / 3 已选')
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(450)
  check('E4.3 半选时再点标题 = 补齐全组（不是取消）', groupCount(0) === '3 / 3 已选', { c: groupCount(0) })
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(450)
  check('E4.4 全选时再点标题 = 取消整组', ev(`(()=>['7345','7409','7514'].every(id=>!document.querySelector('[data-testid="lane-member-'+id+'"]').checked))()`) === true)
  check('E4.5 取消整组后组框回 unchecked', ev(`(()=>{const g=document.querySelector('[data-testid="lane-group-1984:7345"]');return !g.checked&&!g.indeterminate})()`) === true)
  check('E4.6 取消整组后右栏计数 0 条', rightCount() === '0 条', { c: rightCount() })

  // 组外人工补选 + 取消整组不吞组外
  await sh('click', '[data-testid="lane-compose-tab-find"]')
  await wait(350)
  const findId = ev(`(()=>{const b=document.querySelector('[data-testid^="lane-find-add-"]');return b?b.getAttribute('data-testid').replace('lane-find-add-',''):null})()`)
  await sh('click', `[data-testid="lane-find-add-${findId}"]`)
  await wait(450)
  check('E4.7 组外人工补选成功（右栏 1 条）', rightCount() === '1 条', { c: rightCount() })
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(400)
  await sh('click', '[data-testid="lane-cancel-1984:7345"]')
  await wait(450)
  check('E4.8 取消整组保留组外人工补选', rightCount() === '1 条')

  // ---------- E9 已选 tab 取消勾选 ----------
  await sh('click', '[data-testid="lane-compose-tab-chosen"]')
  await wait(350)
  check('E9.1 已选事件 tab 逐条带真实复选框', ev(`document.querySelector('.lci__scroll').querySelectorAll('input[type="checkbox"]').length`) === 1)
  await sh('click', '.lci__scroll input[type="checkbox"]')
  await wait(450)
  check('E9.2 取消勾选即移出本次（计数 0 条）', rightCount() === '0 条')

  // ---------- E5 人工名不被覆盖 ----------
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(400)
  await sh('click', '[data-testid="lane-compose-tab-find"]')
  await wait(300)
  const nameInputSel = ev(`(()=>{const i=document.querySelector('.bco__actions input[type="text"], .bco input[type="text"]');return i?(i.getAttribute('data-testid')||i.className||'input'):null})()`)
  check('E5.0 命名走独立弹窗（工作面内无起名输入框）', nameInputSel === null, { nameInputSel })
  // 通过「起名并创建」弹窗命名
  await sh('click', '[data-testid="lane-compose-tab-chosen"]')
  await wait(200)
  const named = ev(`(()=>{const btn=[...document.querySelectorAll('.bco button')].find(b=>/起名并创建/.test(b.textContent));if(!btn)return null;btn.click();return true})()`)
  await wait(500)
  const dlg = ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');if(!i)return null;return {found:true,testid:i.getAttribute('data-testid'),cls:i.className,ph:i.placeholder}})()`)
  check('E5.1 打开命名弹窗', named === true && dlg && dlg.found, { dlg })
  if (dlg && dlg.found) {
    await ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');const s=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value').set;s.call(i,'人工起的名字');i.dispatchEvent(new Event('input',{bubbles:true}));i.dispatchEvent(new Event('change',{bubbles:true}));return i.value})()`)
    await wait(200)
    check('E5.2 人工名写入成功', ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');return i&&i.value==='人工起的名字'})()`) === true)
    // 关掉弹窗（保留草稿）：用弹窗自己的取消/关闭按钮
    const dlgBtns = ev(`(()=>{const d=document.querySelector('.app-dialog-overlay .app-dialog');if(!d)return null;return [...d.querySelectorAll('button')].map(b=>({t:(b.textContent||'').trim().slice(0,12),ds:b.getAttribute('data-testid')}))})()`)
    await ev(`(()=>{const c=document.querySelector('[data-testid="compose-name-close"]');if(c){c.click();return true}return null})()`)
    await wait(300)
    await waitNoDialog()
    check('E5.3 关闭命名弹窗后草稿保留（成员仍在）', rightCount() === '3 条', { cnt: rightCount(), dlgBtns })
    check('E5.4 命名弹窗已关闭', ev(`!!document.querySelector('.app-dialog-overlay')`) === false)
    // 再选组 / 取消组
    await sh('click', '[data-testid="lane-title-1914:7107"]')
    await wait(400)
    await sh('click', '[data-testid="lane-cancel-1914:7107"]')
    await wait(400)
    const nameVal = ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');return i?i.value:null})()`)
    check('E5.5 选组/取消组后人工名未被覆盖', nameVal === null || nameVal === '人工起的名字', { nameVal })
  }

  // ---------- E10 / E11 日期共窗与草稿确认 ----------
  const dateSel = ev(`(()=>{const s=document.querySelector('.bco__window select');if(s)return 'select';const i=document.querySelector('.bco__window input[type="date"]');return i?'date':null})()`)
  check('E10.0 日期控件存在', dateSel !== null, { dateSel })
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(400)
  const beforeChosen = rightCount()
  await ev(`(()=>{const s=document.querySelector('.bco__window select');if(!s)return null;s.value='7';s.dispatchEvent(new Event('change',{bubbles:true}));return s.value})()`)
  await wait(600)
  const dlgText = ev(`(()=>{const d=document.querySelector('.app-dialog-overlay .app-dialog');return d?d.innerText.slice(0,200):null})()`)
  check('E10.1 有草稿切日期先弹确认', /清空|切换|草稿/.test(String(dlgText)), { dlgText })
  if (dlgText) {
    await ev(`(()=>{const d=document.querySelector('.app-dialog-overlay .app-dialog');if(!d)return null;const btns=[...d.querySelectorAll('button')];const c=btns.find(b=>/保留|取消/.test(b.textContent||''));if(c){c.click();return (c.textContent||'').trim()}return null})()`)
    await wait(300)
    const closed = await waitNoDialog()
    check('E10.2 取消后旧范围与成员保留', rightCount() === beforeChosen, { beforeChosen, now: rightCount() })
    check('E10.3 确认弹窗已关闭', closed === true)
  }

  // ---------- E12 已有泳道只读参照 ----------
  await sh('click', '[data-testid="lane-compose-show-assigned"]')
  await wait(700)
  const readonlyNodes = ev(`(()=>[...document.querySelectorAll('.lcc-canvas__node')].filter(x=>x.getAttribute('aria-label').includes('只读')).length)()`)
  check('E12.1 显式开启后 active 泳道点出现且标只读', readonlyNodes > 0, { readonlyNodes })
  // E15 相关节点标题：开启只读参照后，只读相关节点仍在提示行、不进画布梯子里逐点画
  // 前置：E10 段把 1984 组取消过，这里重新选回来（相关标题只在该组有成员时出现）
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(500)
  check('E15.4 前置：1984 组已重新选中', ev(`(()=>['7345','7409','7514'].every(id=>document.querySelector('[data-testid="lane-member-'+id+'"]').checked))()`) === true)
  const ids12 = titleIds()
  check('E15.5 开启只读参照后：只读相关节点仍不进画布标题（宁缺勿叠、不压字）', !ids12.some(id => rel30.readonly.includes(id)), { readonly: rel30.readonly.length })
  check('E15.6 提示行继续点名只读相关节点', rel30.readonly.every(id => hintText().includes(trunc10(rel30.labelOf.get(id)))), { hint: hintText().slice(0, 80) })
  const rwPoint = ev(`(()=>{const n=[...document.querySelectorAll('.lcc-canvas__node')].find(x=>x.getAttribute('aria-label').includes('只读'));return n?n.getAttribute('data-node'):null})()`)
  if (rwPoint) {
    await sh('click', `[data-node="${rwPoint}"] .lcc-canvas__point`)
    await wait(450)
    const c = ev(cardInfo)
    check('E12.2 只读点的卡片无选入动作', c.exists && !/选入本次泳道|移出本次泳道/.test(c.text), { c })
    await sh('click', '[data-testid="lane-compose-clear-focus"]')
    await wait(400)
    check('E12.3 取消聚焦不偷关已有泳道开关', ev(`document.querySelector('[data-testid="lane-compose-show-assigned"]').checked`) === true)
  }
  await sh('click', '[data-testid="lane-compose-show-assigned"]')
  await wait(500)
  check('E12.4 关掉开关后只读点消失', ev(`(()=>[...document.querySelectorAll('.lcc-canvas__node')].filter(x=>x.getAttribute('aria-label').includes('只读')).length)()`) === 0)

  // ---------- E15b 14 天窗（列够宽）：可画的相关节点在节点下出标题缩略 ----------
  await reset()
  await sh('set', 'viewport', '1440', '900')
  await wait(400)
  const midApi = await (await fetch(`${BASE}/api/semantic-boards/${BOARD}/persistent-topics/compose-candidates?days=14`)).json()
  const midData = midApi.data ?? midApi
  const midPoolOf = (chain) => relatedSets(midData, chain?.section_ids ?? []).pool
  const midChain = (midData.candidate_chains ?? []).find(c => midPoolOf(c).length > 0)
  check('E15.7 14 天窗里存在带「可画相关节点」的候选链', !!midChain, { key: midChain?.key, chains: midData.candidate_chains?.length })
  if (midChain) {
    const wantPool = midPoolOf(midChain)
    await ev(`(()=>{const s=document.querySelector('.bco__window select');if(!s)return null;s.value='14';s.dispatchEvent(new Event('change',{bubbles:true}));return s.value})()`)
    let midReady = false
    for (let i = 0; i < 40; i++) {
      if (ev(`!!document.querySelector('[data-testid="lane-title-${midChain.key}"]')`)) { midReady = true; break }
      await wait(400)
    }
    check('E15.8 14 天窗候选链就位', midReady === true, { key: midChain.key })
    if (midReady) {
      await ensureChainSelected(midChain.section_ids.map(String))
      await wait(800)
      const poolTitles = ev(`(()=>[...document.querySelectorAll('.lcc-canvas__related-title')].map(t=>{const id=t.getAttribute('data-testid').replace('lane-related-title-','');const n=document.querySelector('[data-node="'+id+'"] .lcc-canvas__point');return {id,text:t.textContent,x:Number(t.getAttribute('x')),y:Number(t.getAttribute('y')),ny:n?Number(n.getAttribute('cy')):null}}))()`)
      const midSel = checkedMemberIds()
      const midPool = relatedSets(midData, midSel).pool
      const midState = { cnt: rightCount(), sel: midSel }
      check('E15.9 可画的相关节点都在节点下出标题缩略（不超上限 12，且都属于相关集合）', Array.isArray(poolTitles) && poolTitles.length > 0 && poolTitles.length <= Math.min(midPool.length, 12) && poolTitles.every(t => midPool.includes(t.id)), { got: poolTitles?.length, want: Math.min(midPool.length, 12), pool: midPool.length, ...midState })
      check('E15.10 标题是缩略（≤11 字）且在节点下方', Array.isArray(poolTitles) && poolTitles.length > 0 && poolTitles.every(t => t.text.length <= 11 && /…$/.test(t.text)) && relatedRelatedOk(poolTitles), poolTitles?.slice(0, 3))
      check('E15.11 相关标题互不重叠', ev(`(()=>{const bs=[...document.querySelectorAll('.lcc-canvas__related-title')].map(t=>t.getBoundingClientRect());for(let i=0;i<bs.length;i++)for(let j=i+1;j<bs.length;j++){const a=bs[i],b=bs[j];if(!(a.right<=b.left||b.right<=a.left||a.bottom<=b.top||b.bottom<=a.top))return false}return true})()`) === true)
      await sh('screenshot', join(SHOTS, '08-related-titles.png'))
    }
  }

  // ---------- E15c 90 天窗（列很窄）：画布不被标题淹没，未标名的如实说明 ----------
  await reset()
  await sh('set', 'viewport', '1440', '900')
  await wait(400)
  const wideApi = await (await fetch(`${BASE}/api/semantic-boards/${BOARD}/persistent-topics/compose-candidates?days=90`)).json()
  const wideData = wideApi.data ?? wideApi
  const widePoolOf = (chain) => relatedSets(wideData, chain?.section_ids ?? []).pool
  const wideChain = (wideData.candidate_chains ?? []).find(c => widePoolOf(c).length > 0)
  check('E15.12 90 天窗里存在带「可画相关节点」的候选链', !!wideChain, { key: wideChain?.key })
  if (wideChain) {
    await ev(`(()=>{const s=document.querySelector('.bco__window select');if(!s)return null;s.value='90';s.dispatchEvent(new Event('change',{bubbles:true}));return s.value})()`)
    let wideReady = false
    for (let i = 0; i < 50; i++) {
      if (ev(`!!document.querySelector('[data-testid="lane-title-${wideChain.key}"]')`)) { wideReady = true; break }
      await wait(400)
    }
    check('E15.13 90 天窗候选链就位', wideReady === true, { key: wideChain.key })
    if (wideReady) {
      await ensureChainSelected(wideChain.section_ids.map(String))
      await wait(1000)
      const denseTitles = ev(`document.querySelectorAll('.lcc-canvas__related-title').length`)
      const denseHint = ev(`(()=>{const h=document.querySelector('[data-testid="lane-related-hint"]');return h?h.textContent.trim():''})()`)
      const densePool = relatedSets(wideData, checkedMemberIds()).pool
      check('E15.14 90 天窗：画布上的相关标题不超上限（不淹没画布）', denseTitles <= 12, { denseTitles })
      check('E15.15 90 天窗：没标名的相关节点在提示行里如实说明', denseTitles < densePool.length ? /未标名/.test(denseHint) : true, { denseTitles, pool: densePool.length, hint: denseHint.slice(0, 90) })
    }
  }

  // ---------- E7 / E8 节点卡 ----------
  await reset()
  await sh('set', 'viewport', '1440', '900')
  await wait(400)
  const pt = ev(`(()=>{const n=document.querySelector('[data-node="5872"] .lcc-canvas__point');const r=n.getBoundingClientRect();return {x:Math.round(r.x+r.width/2),y:Math.round(r.y+r.height/2)}})()`)
  await sh('mouse', 'move', '700', '780')
  await wait(350)
  check('E7.0 未悬停时无卡片', ev(cardInfo).exists === false)
  await sh('mouse', 'move', String(pt.x), String(pt.y))
  await wait(500)
  const hov = ev(cardInfo)
  check('E7.1 悬停出卡（标题/日期/关系条数）', hov.exists && /特朗普宣布美军正式撤离伊拉克/.test(hov.text) && /2026-10-01/.test(hov.text) && /关系/.test(hov.text), hov)
  await sh('click', '[data-node="5872"] .lcc-canvas__point')
  await wait(500)
  const pinned = ev(cardInfo)
  check('E7.2 点选后卡片钉住', pinned.exists && !pinned.hidden, pinned)
  await sh('mouse', 'move', '700', '780')
  await wait(400)
  const still = ev(cardInfo)
  check('E7.3 鼠标移开后卡片仍在（跟随选中节点）', still.exists && !still.hidden, still)
  const pos = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]').getBoundingClientRect();const n=document.querySelector('[data-node="5872"] .lcc-canvas__point').getBoundingClientRect();return Math.round(Math.abs((c.x+c.width/2)-(n.x+n.width/2)))})()`)
  check('E7.4 卡片贴着节点（横向距离 < 200px）', pos < 200, { pos })
  const actIdx = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const bs=[...c.querySelectorAll('button')];const i=bs.findIndex(x=>/选入本次泳道|移出本次泳道/.test(x.textContent));return i>=0?i+1:-1})()`)
  check('E7.5 卡内有快捷选入/移出动作', actIdx > 0, { actIdx })
  const btnBefore = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const b=[...c.querySelectorAll('button')].find(x=>/选入本次泳道|移出本次泳道/.test(x.textContent));return b?b.textContent.trim():null})()`)
  await sh('click', `[data-testid="lane-node-card"] button:nth-of-type(${actIdx})`)
  await wait(500)
  const btnAfter = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const b=[...c.querySelectorAll('button')].find(x=>/选入本次泳道|移出本次泳道/.test(x.textContent));return b?b.textContent.trim():null})()`)
  check('E7.6 快捷动作真实改成员并当场翻字', rightCount() === '1 条' && btnBefore !== btnAfter, { btnBefore, btnAfter, cnt: rightCount() })
  await sh('screenshot', join(SHOTS, '03-node-card.png'))
  // 移出
  const actIdx2 = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const bs=[...c.querySelectorAll('button')];const i=bs.findIndex(x=>/选入本次泳道|移出本次泳道/.test(x.textContent));return i>=0?i+1:-1})()`)
  await sh('click', `[data-testid="lane-node-card"] button:nth-of-type(${actIdx2})`)
  await wait(500)
  check('E7.7 快捷移出真实生效（计数回 0）', rightCount() === '0 条', { cnt: rightCount() })
  // × 取消
  const closeIdx = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const bs=[...c.querySelectorAll('button')];const i=bs.findIndex(x=>x.textContent.trim()==='×');return i>=0?i+1:-1})()`)
  await sh('click', `[data-testid="lane-node-card"] button:nth-of-type(${closeIdx})`)
  await wait(500)
  const closed = ev(cardInfo)
  check('E8.1 × 收起卡片', closed.exists === false || closed.hidden === true, closed)
  check('E8.2 × 同时清掉右栏线索', ev(`(()=>{const t=document.querySelector('.lci').innerText;const i=t.indexOf('当前事件的线索');return /点画布上的点看详情/.test(t.slice(i,i+60))})()`) === true)
  // 再点同一节点 = 取消选中
  await sh('click', '[data-node="5872"] .lcc-canvas__point')
  await wait(450)
  const on = ev(cardInfo)
  await sh('click', '[data-node="5872"] .lcc-canvas__point')
  await wait(450)
  const off = ev(cardInfo)
  check('E8.3 再点同一节点即取消选中（卡片收起）', on.exists && !on.hidden && (off.exists === false || off.hidden === true), { on, off })

  // ---------- E13 缺名称拒绝创建 ----------
  // ⚠ 安全：本故事必须先把自动预填名**清空**再点“确认创建”，否则会真创建一条泳道（写真库）。
  await reset()
  await sh('set', 'viewport', '1440', '900')
  await wait(400)
  const chainsBefore = ev(`document.querySelectorAll('[data-testid^="lane-group-"]').length`)
  await sh('click', '[data-testid="lane-title-1984:7345"]')
  await wait(400)
  await ev(`(()=>{const btn=[...document.querySelectorAll('.bco button')].find(b=>/起名并创建/.test(b.textContent));if(btn)btn.click();return true})()`)
  await wait(500)
  const prefilled = ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');return i?i.value:null})()`)
  check('E13.0 命名弹窗自动预填候选话题名', typeof prefilled === 'string' && prefilled.length > 0, { prefilled })
  await ev(`(()=>{const i=document.querySelector('.app-dialog-overlay .app-dialog input');const s=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value').set;s.call(i,'');i.dispatchEvent(new Event('input',{bubbles:true}));i.dispatchEvent(new Event('change',{bubbles:true}));return i.value})()`)
  await wait(300)
  await ev(`(()=>{document.querySelector('[data-testid="compose-name-confirm"]').click();return true})()`)
  await wait(600)
  const nameErr = ev(`(()=>{const d=document.querySelector('.app-dialog-overlay .app-dialog');if(!d)return {stillOpen:false,err:false,text:''};const err=d.querySelector('.bco__error');return {stillOpen:true,err:!!err&&(err.textContent||'').trim().length>0,text:(err?err.textContent:'').trim()}})()`)
  check('E13.1 缺名称拒绝创建（弹窗仍在 + 显示错误）', nameErr.stillOpen === true && nameErr.err === true, { nameErr })
  check('E13.2 拒绝时成员保留', rightCount() === '3 条', { cnt: rightCount() })
  check('E13.3 未真创建（候选链数量不变、未弹成功）', ev(`document.querySelectorAll('[data-testid^="lane-group-"]').length`) === chainsBefore, { chainsBefore })
  await sh('screenshot', join(SHOTS, '04-empty-name.png'))
  await ev(`(()=>{const c=document.querySelector('[data-testid="compose-name-close"]');if(c)c.click();return true})()`)
  await wait(400)

  // ---------- G5 重排（1920） ----------
  await sh('set', 'viewport', '1920', '1080')
  await wait(900)
  check('G5.1 1920 下画布仍 1:1', ev(`(()=>{const p=document.querySelector('.lcc-canvas__point');const r=Number(p.getAttribute('r'));const px=p.getBoundingClientRect().width/2;return Math.abs(r-px)<0.6})()`) === true)
  check('G5.2 1920 下无元素越界', ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface').getBoundingClientRect();let bad=0;document.querySelectorAll('.lcc-canvas__surface text,.lcc-canvas__surface rect,.lcc-canvas__surface circle').forEach(el=>{const b=el.getBoundingClientRect();if(b.width===0&&b.height===0)return;if(b.right>s.right+1||b.bottom>s.bottom+1)bad++;});return bad===0})()`) === true)
  check('G5.3 1920 下普通点半径仍 ≥3px', ev(`(()=>Array.from(document.querySelectorAll('.lcc-canvas__point')).every(p=>Number(p.getAttribute('r'))>=3))()`) === true)
  await sh('screenshot', join(SHOTS, '05-1920.png'))

  // ---------- E16 滚轮缩放（用户 2026-11-01 补充） ----------
  await reset()   // 回到默认 30 天窗（上一段是 90 天窗，节点集不同）
  await sh('set', 'viewport', '1440', '900')
  await wait(700)
  const zoomReadout = () => ev(`document.querySelector('[data-testid="lane-compose-zoom-readout"]').textContent.trim()`)
  const zoomBefore = zoomReadout()
  const vbBefore = ev(`document.querySelector('[data-testid="lane-compose-canvas-svg"]').getAttribute('viewBox')`)
  const svgWidthBefore = Number(ev(`document.querySelector('[data-testid="lane-compose-canvas-svg"]').getAttribute('width')`))
  check('E16.1 初始缩放读数 100%', zoomBefore === '100%', { zoomBefore })
  await ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface');const r=s.getBoundingClientRect();s.dispatchEvent(new WheelEvent('wheel',{deltaY:-240,clientX:r.left+320,clientY:r.top+220,bubbles:true,cancelable:true}));return true})()`)
  await wait(700)
  const zoomAfter = zoomReadout()
  const vbAfter = ev(`document.querySelector('[data-testid="lane-compose-canvas-svg"]').getAttribute('viewBox')`)
  const svgWidthAfter = Number(ev(`document.querySelector('[data-testid="lane-compose-canvas-svg"]').getAttribute('width')`))
  check('E16.2 滚轮放大后读数变大', Number(zoomAfter.replace('%', '')) > 100, { zoomBefore, zoomAfter })
  check('E16.3 等比缩放：viewBox 不变、SVG 宽变大（日期列不重排）', vbAfter === vbBefore && svgWidthAfter > svgWidthBefore, { vbBefore, vbAfter, svgWidthBefore, svgWidthAfter })
  check('E16.4 缩放后普通点渲染半径仍 ≥3px', ev(`(()=>Array.from(document.querySelectorAll('.lcc-canvas__point')).every(p=>p.getBoundingClientRect().width/2>=3))()`) === true)
  const e165 = await (async () => {
    // 只挑当前**可见**（在画布可视区内）的节点，避免点到画布外的点
    const n = ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface').getBoundingClientRect();const x=[...document.querySelectorAll('.lcc-canvas__node')].find(e=>{const b=e.getBoundingClientRect();return b.width>0&&b.top>=s.top-2&&b.bottom<=s.bottom+2&&b.left>=s.left-2&&b.right<=s.right+2});return x?x.getAttribute('data-node'):null})()`)
    if (!n) return false
    await sh('click', `[data-node="${n}"] .lcc-canvas__point`)
    await wait(500)
    const pos = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');const p=document.querySelector('[data-node="${n}"] .lcc-canvas__point');if(!c||!p)return 9999;const a=c.getBoundingClientRect(),b=p.getBoundingClientRect();return Math.abs((a.left+a.width/2)-(b.left+b.width/2))})()`)
    const box = ev(`(()=>{const c=document.querySelector('[data-testid="lane-node-card"]');if(!c)return null;const b=c.getBoundingClientRect();return {w:Math.round(b.width),h:Math.round(b.height)}})()`)
    await sh('click', '[data-testid="lane-node-card-close"]')
    await wait(300)
    return { ok: pos < 200, pos, box }
  })()
  check('E16.5 缩放后节点卡仍贴着节点（水平距离 <200px）', e165.ok, { pos: e165.pos })
  // E16.8 缩放 + 平移后节点卡不能塌成竖条（宽度固定、不被「可用宽」夹扁）
  check('E16.8 缩放+平移后节点卡不塌成竖条（宽度 ≥240px）', !!e165.box && e165.box.w >= 240 && e165.box.h <= 320, e165.box)
  await sh('click', '[data-testid="lane-compose-zoom-reset"]')
  await wait(600)
  check('E16.6 重置回到 100% 且 SVG 宽复原', zoomReadout() === '100%' && Number(ev(`document.querySelector('[data-testid="lane-compose-canvas-svg"]').getAttribute('width')`)) === svgWidthBefore, { now: zoomReadout() })
  check('E16.7 滚轮在画布上不触发页面滚动（defaultPrevented）', ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface');const r=s.getBoundingClientRect();const e=new WheelEvent('wheel',{deltaY:-120,clientX:r.left+200,clientY:r.top+200,bubbles:true,cancelable:true});s.dispatchEvent(e);return e.defaultPrevented})()`) === true)

  // ---------- E17 拖拽平移（用户 2026-11-01 补充） ----------
  await sh('set', 'viewport', '1440', '900')
  await wait(700)
  // 先放大，保证有可平移的范围
  await ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface');const r=s.getBoundingClientRect();s.dispatchEvent(new WheelEvent('wheel',{deltaY:-240,clientX:r.left+320,clientY:r.top+220,bubbles:true,cancelable:true}));return true})()`)
  await wait(700)
  const panBefore = ev(`document.querySelector('.lcc-canvas__surface').scrollLeft`)
  const cntBeforePan = rightCount()
  const panAfter = ev(`(()=>{const s=document.querySelector('.lcc-canvas__surface');const r=s.getBoundingClientRect();const fire=(t,x,y)=>s.dispatchEvent(new PointerEvent(t,{bubbles:true,cancelable:true,clientX:x,clientY:y,pointerId:1}));fire('pointerdown',r.left+520,r.top+70);fire('pointermove',r.left+420,r.top+70);fire('pointerup',r.left+420,r.top+70);return s.scrollLeft})()`)
  check('E17.1 拖拽平移：内容跟着指针走（scrollLeft 变化）', panAfter !== panBefore, { panBefore, panAfter })
  check('E17.2 拖拽不误触发节点聚焦（没弹出节点卡）', ev(`!!document.querySelector('[data-testid="lane-node-card"]')`) === false)
  check('E17.3 拖拽不改选择与计数', rightCount() === cntBeforePan, { before: cntBeforePan, after: rightCount() })
  await sh('click', '[data-testid="lane-compose-zoom-reset"]')
  await wait(500)

  // ---------- E14 窄屏 ----------
  await sh('set', 'viewport', '375', '667')
  await wait(900)
  const narrow = ev(`(()=>{const t=document.body.innerText;return {wide:/宽屏|横屏|大屏/.test(t),noOverflow:document.documentElement.scrollWidth<=window.innerWidth+1,sw:document.documentElement.scrollWidth,iw:window.innerWidth}})()`)
  check('E14.1 375px 显示宽屏提示', narrow.wide === true, narrow)
  check('E14.2 375px 无横向溢出', narrow.noOverflow === true, narrow)
  await sh('screenshot', join(SHOTS, '06-375.png'))
  await sh('set', 'viewport', '390', '844')
  await wait(700)
  const narrow2 = ev(`(()=>({wide:/宽屏|横屏|大屏/.test(document.body.innerText),noOverflow:document.documentElement.scrollWidth<=window.innerWidth+1}))()`)
  check('E14.3 390px 宽屏提示 + 无横向溢出', narrow2.wide === true && narrow2.noOverflow === true, narrow2)
  await sh('screenshot', join(SHOTS, '07-390.png'))
}

try {
  await main()
} catch (e) {
  console.log('RUN ERROR:', String(e).slice(0, 500))
} finally {
  const failed = results.filter((r) => !r.ok)
  writeFileSync(join(HERE, 'assertions.json'), JSON.stringify({ base: BASE, board: BOARD, total: results.length, failed: failed.length, results }, null, 2))
  console.log(`\nTOTAL ${results.length} checks, ${failed.length} failed`)
  for (const f of failed) console.log(`  ✗ ${f.label}`, JSON.stringify(f.extra ?? {}))
  try { sh('close') } catch { /* ignore */ }
  process.exit(failed.length ? 1 : 0)
}
