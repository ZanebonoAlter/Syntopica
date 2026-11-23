# Review · lane-compose-topic-event-tags

- 审查日期：2026-11-23
- 审查方式：`change-review` 子线程（只读），范围 = 本 change 12 个代码文件 + 3 份文档（工作树并发脏文件不审）
- 结论：**有条件通过** → 11 项发现全部处置完毕（6 必修 + 5 Low）

### R-1 2026-11-23【Medium】delta 需求正文新增「拖出窗口」取消语义未实现，且与主 spec「移出可撤销」冲突
- **状态**：已修复
- 处置：delta 正文改为「拖出本次泳道行松手 = 移出，沿用既有『移出可撤销』口径」；取消语义只保留 Esc / `pointercancel` / 指针捕获丢失。

### R-2 2026-11-23【Medium】proposal 的 Capabilities / Non-Goals 与实际 delta 与实现矛盾
- **状态**：已修复
- 处置：Capabilities 改为「新增 3 条 requirement + 修改 1 条既有 requirement」；Non-Goals 右栏措辞改为「不新增段落/开关，仅②条目加 ★ 字符」；design D6 同步。

### R-3 2026-11-23【Medium】proposal 头部注释区位置不合仓库约定 → ui-verification-trace fail-open
- **状态**：已修复
- 处置：`complexity` / `ui-impact` / `constraint-domains` 三行移到标题之前，并补 `review:` 指向本文件；重跑 `ui-verification-trace.sh` 走真实 minor 判定。

### R-4 2026-11-23【Medium】design D8 的「4.5px 可辨认下限」未实现，ui-verification 误述为已实现
- **状态**：已采纳（选方案 b：统一文档口径，不实现星形专属下限）
- 处置：design D8/Risks、ui-design、interaction-table S-9、ui-verification 残余风险统一改为「沿用通用 3px 像素下限；50% 下辨识度偏低属已知取舍，不放大外接半径（放大=伪造接近度）」。

### R-5 2026-11-23【Medium】拖拽「起点必须取 pointerdown 位置」的核心不变量没有断言
- **状态**：已修复
- 处置：`LaneComposeCanvas.test.ts` 改为数值断言 `translate(24, 18)`（首帧 move 起点的回归会立即红）。

### R-6 2026-11-23【Low】tasks 三条验证项未勾选、stage 停在 design、review 建账缺失
- **状态**：已修复
- 处置：tasks 4.4/5.5/6.8 全部勾选并写实测结果；截图归档到 `ui-verification/`；stage 推进；本文件建账。

### R-7 2026-11-23【Low】★ 的两条 spec 明文要求没有落到测试
- **状态**：已修复（几何部分改为容差口径）
- 处置：补 aria-label「已确认关键线索 2 条」断言；spec「不溢出」改述为「不明显溢出（描边半宽容差内）」。

### R-8 2026-11-23【Low】星形外接尺寸断言依赖两节点半径相同，挡不住「星形半径写死」
- **状态**：已修复
- 处置：改为**同一节点**带星/不带星两次渲染对照（先挂对照圆点取 r，再挂星形量 bbox）。

### R-9 2026-11-23【Low】两份 flow/api 文档混有别的 change 的改动
- **状态**：已采纳（并发工作树现实）
- 处置：commit 时按文件整体暂存，commit message 显式说明「docs 含并发 change 的 hunk」；不在本 change 内拆分他 change 内容。

### R-10 2026-11-23【Low】api 文档「可为空」与实际序列化形态（省略键）有出入
- **状态**：已修复
- 处置：文档改为「为空时该键省略（`omitempty`）」。

### R-11 2026-11-23【Low】`pointerdown` 的 preventDefault 取消了节点原生聚焦
- **状态**：已采纳（记录边界）
- 处置：接受该行为（节点卡由 `focusId` 驱动，键盘 Tab/Enter 不受影响），在 ui-design 里写明；不额外加手动 focus 以免与拖拽手势抢事件。

## 复核命令与结果

| 命令 | 结果 |
|---|---|
| `backend-go go test ./internal/topicgraph/{repository,handler} -run 'ComposeCandidates\|KeyEvent' -count=1` | ok |
| `front pnpm test:unit persistentTopics / LaneComposeCanvas / LaneComposeInspector / BoardComposeOverlay --maxWorkers=2` | 全绿 |
| `bash scripts/harness/ui-verification-trace.sh <change>` | 见归档门禁实测 |
| `bash scripts/harness/review-trace.sh <change>` | 本文件建账后通过 |
