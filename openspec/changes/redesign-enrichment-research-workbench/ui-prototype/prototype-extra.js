/* 可丢弃规划原型：只改浏览器内存，没有 API、持久化、LLM 或真实任务。 */
const prototypeMode = new URLSearchParams(location.search).get('demo') || '';
let activeDemo = prototypeMode;
let utilityMode = '';
let utilityTopic = '';
let backgroundText = '示例新闻记忆：围绕通航、谈判与政策行动的公开报道持续累积。这里只记录新闻背景，不写入研究报告的主观判断。';
const versions = new Map([['mandeb', 2]]);
const viewedVersions = new Map();

const extraStyle = document.createElement('style');
extraStyle.textContent = `
  .filter-count{display:none}.filters{gap:15px;overflow:auto;flex-shrink:0}
  .line-title{display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
  .extra-actions{display:flex;align-items:center;gap:14px;flex-shrink:0}
  .extra-actions select{background:var(--color-bg-sunken);color:var(--color-text-secondary);border:1px solid var(--color-border-medium);border-radius:4px;font-size:11px;padding:4px}
  .extra-actions button{font-size:11px}.extra-actions .muted{font-size:10px}
  .index-foot button{font-size:11px;margin-top:7px;color:var(--color-accent)}
  .task-link{color:var(--color-accent);background:transparent;font-size:11px;padding:0}
  .board-nav button{padding:0;background:none;color:var(--color-text-muted);font-size:12px;white-space:nowrap}
  .utility-dialog{padding:24px;overflow:auto}.utility-dialog[data-size=sm]{width:min(420px,92vw)}
  .utility-dialog[data-size=lg]{width:min(760px,92vw)}.utility-dialog[data-size=xl]{width:min(1040px,92vw)}
  .utility-body{font-size:13px}.utility-body button{font-size:12px}.utility-body select,.utility-body input,.utility-body textarea{font:inherit;color:var(--color-text-primary);background:var(--color-bg-sunken);border:1px solid var(--color-border-medium);border-radius:4px;padding:8px;max-width:100%}
  .utility-body textarea{display:block;width:100%;min-height:150px;line-height:1.9;resize:vertical}
  .utility-tools{display:flex;gap:12px;flex-wrap:wrap;align-items:center;margin:15px 0}
  .utility-tools label{display:flex;gap:8px;align-items:center;color:var(--color-text-secondary);font-size:12px}
  .legacy-layout{display:grid;grid-template-columns:190px minmax(0,1fr);gap:24px;margin-top:20px}
  .legacy-tabs{display:flex;flex-direction:column;align-items:stretch;gap:6px}
  .legacy-tabs button{text-align:left;color:var(--color-text-secondary);padding:10px;background:var(--color-bg-sunken);border-radius:4px}
  .legacy-tabs button[aria-pressed=true]{color:var(--color-accent);background:var(--color-accent-subtle)}
  .legacy-content{min-width:0;max-height:54dvh;overflow:auto}.legacy-content h3{font-size:20px;margin:0 0 12px}.legacy-content p{line-height:1.95}
  .readonly-label{color:var(--color-accent);font-size:11px;border-bottom:1px solid var(--color-border-subtle);padding-bottom:12px}
  .prototype-alert{padding:22px;color:var(--color-text-secondary);font-size:13px;line-height:1.9}
  .prototype-alert strong{color:var(--color-text-primary);display:block;margin-bottom:8px}
  @media(max-width:767.98px){.extra-actions{gap:9px}.extra-actions select{max-width:88px}.extra-actions .detail-status-label{display:none}.extra-actions button{font-size:10px}.board-nav button{font-size:11px}.legacy-layout{display:block}.legacy-tabs{flex-direction:row;overflow:auto;margin-bottom:18px}.legacy-tabs button{white-space:nowrap}.legacy-content{max-height:48dvh}.utility-dialog{padding:20px}}
`;
document.head.append(extraStyle);

const ignoredFilter = document.createElement('button');
ignoredFilter.type = 'button';
ignoredFilter.className = 'filter';
ignoredFilter.dataset.filter = 'ignored';
ignoredFilter.setAttribute('aria-pressed', 'false');
ignoredFilter.textContent = '已忽略';
$('.filters').append(ignoredFilter);
$('[data-filter="pending"]').firstChild.textContent = '未研究';

const taskLink = document.createElement('button');
taskLink.type = 'button';
taskLink.className = 'task-link';
taskLink.hidden = true;
$('.page-title-row').append(taskLink);

const toolbarStatus = $('#detail-status');
const extraActions = document.createElement('div');
extraActions.className = 'extra-actions';
toolbarStatus.replaceWith(extraActions);
extraActions.append(toolbarStatus);
toolbarStatus.className = 'detail-status-label';
const versionSelect = document.createElement('select');
versionSelect.setAttribute('aria-label', '报告版本');
const regenerateButton = document.createElement('button');
regenerateButton.type = 'button';
regenerateButton.className = 'ghost-btn';
regenerateButton.textContent = '重新研究';
const dispositionButton = document.createElement('button');
dispositionButton.type = 'button';
dispositionButton.id = 'disposition-action';
dispositionButton.className = 'ghost-btn';
extraActions.append(versionSelect, regenerateButton, dispositionButton);
const historyButton = document.createElement('button');
historyButton.type = 'button';
historyButton.id = 'history-entry';
historyButton.className = 'ghost-btn';
historyButton.textContent = '历史研究（只读） ↗';
$('.index-foot').append(document.createElement('br'), historyButton);

const utilityDialog = document.createElement('dialog');
utilityDialog.id = 'utility-dialog';
utilityDialog.className = 'utility-dialog';
utilityDialog.setAttribute('aria-labelledby', 'utility-title');
utilityDialog.innerHTML = '<div class="dialog-head"><span class="dialog-kicker" id="utility-kicker">补充交互 · 示例数据</span><button class="ghost-btn" id="utility-close" type="button" aria-label="关闭补充视图">关闭 ×</button></div><h2 id="utility-title"></h2><div class="utility-body" id="utility-body"></div>';
document.body.append(utilityDialog);
$('#utility-close').onclick = () => utilityDialog.close();
const runningItem = () => items.find(item => item.status === 'running' || item.regenerating);

function showUtility(kind, item = current()) {
  utilityMode = kind;
  utilityDialog.dataset.size = kind === 'history' ? 'xl' : kind === 'regenerate' || kind === 'task' ? 'sm' : 'lg';
  const body = $('#utility-body');
  if (kind === 'history') {
    $('#utility-title').textContent = '历史研究';
    body.innerHTML = '<div class="readonly-label">旧版产物 · 只读 · 不启动旧生成、追问、沉淀或评审</div><div class="legacy-layout"><nav class="legacy-tabs" aria-label="旧产物类型"><button type="button" data-legacy="board" aria-pressed="true">旧版板块报告</button><button type="button" data-legacy="topic" aria-pressed="false">聚焦分析</button><button type="button" data-legacy="debate" aria-pressed="false">个股辩论</button><button type="button" data-legacy="qa" aria-pressed="false">已有问答</button><button type="button" data-legacy="review" aria-pressed="false">旧评审记录</button></nav><article class="legacy-content" id="legacy-content"></article></div>';
    renderLegacy('board');
  } else if (kind === 'background') {
    utilityTopic = item?.topic || '霍尔木兹海峡通航';
    $('#utility-title').textContent = utilityTopic + ' · 新闻背景';
    body.innerHTML = `<div class="readonly-label">话题上下文示意 · 实际从话题总览选中此话题后打开，不依赖聚焦分析</div><div class="utility-tools"><label>粒度 <select id="background-gran"><option value="month">月</option><option value="year">年</option><option value="week">周</option></select></label><label>周期 <input id="background-period" aria-label="背景周期" type="month" value="2026-10"></label></div><p id="background-content">${escape(backgroundText)}</p><textarea id="background-draft" aria-label="编辑新闻背景" hidden>${escape(backgroundText)}</textarea><div class="utility-tools"><button class="ghost-btn" type="button" data-background="edit">编辑背景</button><button class="primary-btn" type="button" data-background="save" hidden>保存编辑</button><button class="ghost-btn" type="button" data-background="cancel" hidden>取消编辑</button><button class="ghost-btn" type="button" data-background="prepare">补生成 / 重新汇总</button></div><div class="boundary-note" id="background-confirm" hidden><b>确认生成选定周期背景？</b><p>正式产品会调用 AI，已存在周期会覆盖新闻背景；不会改写研究报告。当前原型只演示确认。</p><button class="primary-btn" type="button" data-background="generate">确认生成（演示）</button> <button class="ghost-btn" type="button" data-background="cancel-generate">取消</button></div><p class="muted">浏览与编辑读取不触发研究；生成需再次明确确认。示例更改仅留浏览器内存。</p>`;
  } else if (kind === 'regenerate') {
    $('#utility-title').textContent = '确认重新研究？';
    body.innerHTML = '<p>正式操作将重新取数与计算并消耗预算，但仍使用这条候选冻结的新闻快照与 cutoff；新新闻需要重新发现。成功后追加版本，不覆盖正在阅读的报告。</p><div class="utility-tools"><button class="primary-btn" type="button" data-confirm-regenerate>确认重新研究（演示）</button><button class="ghost-btn" type="button" data-close-utility>取消</button></div>';
  } else {
    const task = runningItem();
    $('#utility-title').textContent = '当前任务 · 示例进展';
    body.innerHTML = `<p>${escape(task?.title || '暂无运行中的研究')}</p><p>查看进展不会重新发起研究；忽略不能代替取消任务。</p>${task?.regenerating ? '<button class="primary-btn" type="button" data-complete-regenerate>完成演示重研</button>' : '<button class="ghost-btn" type="button" data-return-running>返回任务所在周期</button>'}`;
  }
  if (!utilityDialog.open) utilityDialog.showModal();
}
function renderLegacy(kind) {
  const content = {
    board: '<h3>地区局势与贸易路径 · 旧版分析</h3><p>这里演示按原有 kind 渲染旧版板块报告，不将它转换成新 signal_report。既有内容和依据保留，缺失字段显示缺失。</p>',
    topic: '<h3>美伊谈判 · 历史聚焦分析</h3><p>旧的见解、依据与条件按原载荷只读展示，不再提供聚焦分析触发按钮。</p>',
    debate: '<h3>既有个股辩论 · 历史记录</h3><details open><summary>历史观点 A（示例）</summary><p>此处为原有角色观点与理由。</p></details><details><summary>历史观点 B（示例）</summary><p>此处为原有竞争解释。</p></details><p>只读查看六角色已有输出；不重新辩论。</p>',
    qa: '<h3>已有问答</h3><p><strong>历史问题：</strong>这项判断依赖哪些条件？</p><p><strong>历史回答：</strong>这里按原有对话和引用呈现，不新增输入框，也不重新调用模型。</p>',
    review: '<h3>旧评审记录 · 历史元数据</h3><p>旧结果当时的对比与处理记录保留只读。它不是新报告的评审链，没有新的采纳或驳回动作。</p>'
  };
  $('#legacy-content').innerHTML = content[kind] + '<p class="muted">所有内容为界面示例，未读取真实历史数据。</p>';
  document.querySelectorAll('[data-legacy]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.legacy === kind)));
}

const originalRender = render;
render = function renderPrototype() {
  originalRender();
  const item = current();
  const task = runningItem();
  ignoredFilter.setAttribute('aria-pressed', String(state.filter === 'ignored'));
  $('#line-count').textContent = '已加载 ' + visibleItems().length + ' 条';
  dispositionButton.hidden = !item;
  dispositionButton.disabled = !item || item.status === 'running' || !!item.regenerating;
  dispositionButton.textContent = ignoredCandidates.has(item?.id) ? '恢复线索' : '忽略线索';
  dispositionButton.title = dispositionButton.disabled ? '研究中不可忽略；不会取消任务' : '只处理当前候选，不删除报告或屏蔽话题';
  if (ignoredCandidates.has(item?.id)) document.querySelectorAll('[data-start]').forEach(button => { button.disabled = true; button.title = '先恢复线索再发起研究，已有报告仍可读'; });
  taskLink.hidden = !task;
  taskLink.textContent = task ? '研究中 · 查看任务' : '';
  $('#discover').disabled = !!task || activeDemo === 'disabled';
  regenerateButton.hidden = !item || item.status !== 'ready' || ignoredCandidates.has(item.id);
  regenerateButton.disabled = !!task || activeDemo === 'disabled';
  versionSelect.hidden = !item || item.status !== 'ready';
  if (item?.status === 'ready') {
    const count = versions.get(item.id) || 1;
    versionSelect.innerHTML = Array.from({length:count}, (_, index) => `<option value="${count-index}">版本 ${count-index}${index ? ' · 历史' : ' · 最新'}</option>`).join('');
    versionSelect.value = String(viewedVersions.get(item.id) || count);
  }
  if (activeDemo === 'disabled') {
    document.querySelectorAll('[data-start]').forEach(button => { button.disabled = true; });
    $('#detail-status').textContent = '未开启研究 · 历史仍可读';
  }
  if (activeDemo === 'loading' || activeDemo === 'error' || activeDemo === 'empty') {
    const text = activeDemo === 'loading' ? '线索加载中…' : activeDemo === 'error' ? '列表加载失败，不是没有线索。' : '这个周期还没有研究线索。';
    $('#line-list').innerHTML = `<li class="prototype-alert" ${activeDemo === 'error' ? 'role="alert"' : 'role="status"'}><strong>${text}</strong>${activeDemo === 'error' ? '<button type="button" class="ghost-btn" data-retry-list>重新加载列表</button>' : activeDemo === 'empty' ? '可以显式发现线索，不会自动研究。' : '请稍候，不清除后台任务。'}</li>`;
    $('#line-count').textContent = activeDemo === 'empty' ? '无候选' : activeDemo === 'error' ? '加载失败' : '加载中';
    $('#paper').innerHTML = '<p class="muted">选择一条可用线索后显示详情；错误与空态分开。</p>';
    dispositionButton.hidden = true; versionSelect.hidden = true; regenerateButton.hidden = true;
    toolbarStatus.textContent = activeDemo === 'error' ? '加载失败' : activeDemo === 'loading' ? '加载中' : '无候选';
  }
  if (activeDemo === 'bad-report') $('#paper').innerHTML = '<div class="prototype-alert" role="alert"><strong>这份报告的格式无法读取</strong>保留原数据，请查看其他版本或原始记录。不会自动调用 AI 修复，也不会当作空报告。</div>';
};

dispositionButton.onclick = () => {
  const item = current();
  if (!item || item.status === 'running' || item.regenerating) return;
  if (ignoredCandidates.has(item.id)) {
    ignoredCandidates.delete(item.id); toast('示例：已恢复这条线索，原报告不变。');
  } else {
    ignoredCandidates.add(item.id); toast('示例：已忽略，可在“已忽略”恢复；报告没有删除。');
  }
  const list = visibleItems();
  if (!list.some(row => row.id === state.activeId)) state.activeId = list[0]?.id || null;
  render();
};
historyButton.onclick = () => showUtility('history');
regenerateButton.onclick = () => showUtility('regenerate');
versionSelect.onchange = () => { viewedVersions.set(state.activeId, Number(versionSelect.value)); toast('版本选择示意：切换不触发研究、不修改历史。正式产品按 result_id 读取对应快照。'); };
taskLink.onclick = () => showUtility('task');
const overviewEntry = [...$('.board-nav').children].find(node => node.textContent === '话题总览');
const backgroundEntry = document.createElement('button');
backgroundEntry.type = 'button'; backgroundEntry.textContent = '话题总览';
backgroundEntry.title = '原型快捷演示：模拟选中话题后打开背景，并非新增导航页';
backgroundEntry.onclick = () => showUtility('background');
overviewEntry.replaceWith(backgroundEntry);
const manageBackground = document.createElement('button');
manageBackground.type = 'button'; manageBackground.className = 'ghost-btn'; manageBackground.textContent = '在话题中管理背景（演示） →';
manageBackground.onclick = () => { $('#topic-dialog').close(); showUtility('background'); };
$('#topic-dialog').append(manageBackground);

utilityDialog.addEventListener('click', event => {
  if (event.target.closest('[data-close-utility]')) utilityDialog.close();
  const legacy = event.target.closest('[data-legacy]'); if (legacy) renderLegacy(legacy.dataset.legacy);
  const action = event.target.closest('[data-background]')?.dataset.background;
  if (action === 'edit') {
    $('#background-content').hidden = true; $('#background-draft').hidden = false;
    $('[data-background=save]').hidden = false; $('[data-background=cancel]').hidden = false;
    $('[data-background=edit]').hidden = true;
  }
  if (action === 'save' || action === 'cancel') {
    if (action === 'save') backgroundText = $('#background-draft').value;
    $('#background-content').textContent = backgroundText; $('#background-content').hidden = false;
    $('#background-draft').value = backgroundText; $('#background-draft').hidden = true;
    $('[data-background=save]').hidden = true; $('[data-background=cancel]').hidden = true;
    $('[data-background=edit]').hidden = false;
  }
  if (action === 'prepare') $('#background-confirm').hidden = false;
  if (action === 'cancel-generate') $('#background-confirm').hidden = true;
  if (action === 'generate') { $('#background-confirm').hidden = true; toast('生成确认演示结束；没有调用 AI，没有改写报告。'); }
  if (event.target.closest('[data-confirm-regenerate]')) {
    current().regenerating = true; utilityDialog.close(); render(); toast('重研状态演示：当前旧版仍可读；从顶部查看任务。');
  }
  if (event.target.closest('[data-complete-regenerate]')) {
    const item = runningItem();
    if (item) { versions.set(item.id, (versions.get(item.id) || 1)+1); item.regenerating = false; }
    utilityDialog.close(); render(); toast('示例新版本已追加，原版本保留。');
  }
  if (event.target.closest('[data-return-running]')) {
    const item = runningItem();
    if (item) { rememberScroll(); state.period = item.period; $('#period').value = item.period; state.activeId = item.id; state.filter = 'all'; state.mobileView = 'detail'; }
    utilityDialog.close(); render();
  }
});
utilityDialog.addEventListener('change', event => {
  if (event.target.id === 'background-gran') {
    const gran = event.target.value; const input = $('#background-period');
    input.type = gran === 'month' ? 'month' : gran === 'week' ? 'week' : 'number';
    input.value = gran === 'month' ? '2026-10' : gran === 'week' ? '2026-W42' : '2026';
  }
});
document.addEventListener('click', event => { if (event.target.closest('[data-retry-list]')) { activeDemo = ''; render(); } });
if (activeDemo === 'running') { items.find(item => item.id === 'strait').status = 'running'; state.activeId = 'strait'; }
if (activeDemo === 'long') items.find(item => item.id === 'strait').title = '长标题示例：' + '需要核对不同时间与来源的新闻表述，避免把多种事实合成一个结论。'.repeat(12);
render();
if (activeDemo === 'history') showUtility('history');
if (activeDemo === 'background') showUtility('background');
