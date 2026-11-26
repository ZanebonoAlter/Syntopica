# Design

## D1 判据反转的真正理由

原判据是「判据成立才画」，理由是「不凑数」。方向没错，但它把默认值设成了「不画」——而人的默认行为就是不做额外的事。

反转后默认值是「画」，例外要写理由。代价是可能出现「为了填 marker 而硬画一张没价值的图」。这个代价比「该画的都没画」小：前者 review 能看到（图在那儿，看一眼就知道敷衍），后者看不见（没有图，也就没人问为什么）。

和 ISS-03 同一条逻辑：**机器管「有没有做」，人管「做得好不好」**。

## D2 为什么不做渲染脚本（推翻了自己一小时前的方案）

在写 proposal 前我先做了可行性实验：

| 路径 | 结果 |
|---|---|
| `agent-browser` / chromium headless 渲染 mermaid → PNG → PIL 裁白边 | **技术可行**（实测 2 张成功） |
| `read` PNG 让图出现在对话里 | **失败**——工具结果里的图片在 pi-web 界面不渲染，用户看不到 |
| 直接在回复里写 ` ```mermaid ` 块 | **成功**——pi-web 自带 mermaid，用户能看到图 |

第一条跑通了就以为方案成立，但第二条才是「用户能不能看到」的判定点。**技术可行 ≠ 达成目的**。

所以最终方案是零脚本：mermaid 源既入档也直接贴进对话。

留一条备注：`mmdc` / chromium 那条路以后如果有「导出成图片文件」的需求（比如贴到 PR、发给人看），仍可用，只是不是当前的主路径。

## D3 marker 二值与「理由」的判据边界

`<!-- diagrams: full(理由) -->` / `<!-- diagrams: none(理由) -->`，机器只判：

1. marker 存在
2. 值是 `full` 或 `none` 之一
3. 括号里理由非空

**不判**：理由是否成立、图是否准确、伪代码是否正确。

这条边界必须写清，否则下一个维护者会想「既然都查了，不如也查查理由合不合理」，最后演变成 LLM 判分——那是另一回事，且会引入不稳定门禁。

## D4 伪代码粒度的判定

「写到函数签名 + 关键分支为止」的含义，用反例说更清楚：

| ✅ 合适 | ❌ 过细 | ❌ 过粗 |
|---|---|---|
| `computeLaneHeat(lane, date) → {heat, clues, stories}`<br>if lanes 为空 → 返回 0 而非 null | 写 `for i in 0..n` 的循环体、变量名 | 「计算热度」 |

判据：**看伪代码能不能发现问题**。能发现「这里漏了空集合分支」「这里两次查询可以合成一次」「这里顺序会导致读到旧值」——就够了。看不到这些，说明写得太粗；写到能直接粘贴编译，说明太细。

## D5 与 `ui-design.md` 的边界

`design-diagram.md`（业务图 + 伪代码的伴生图）与 `ui-design.md`（界面契约）的分工：

- 图回答「数据怎么连、消息怎么走、状态怎么变」
- `ui-design.md` L0 回答「界面上有什么、点了会怎样」

L0 需要图时写**指针**（「版块结构见 `design-diagram.md` 的 ER 图」），不复制内容。复制会漂——上一轮修 spec 过期时已经吃过这个亏。

## D6 向后兼容与门禁范围

- 已归档 change：不追溯。无 marker 视为 legacy，不阻断
- 新 change：`.openspec.yaml` 存在即要求 marker
- 门禁挂在 `archive-readiness` 的第 8 项，不塞进 `check-standards`（后者是仓库级规范检查，前者是 change 生命周期检查）

## D7 样板选择

给已归档的 change 补 `design-diagram.md` 作样板。选 `2026-11-26-merge-lane-ops-specs`：

- 刚归档，结构清楚
- 有真实 ER 关系（spec 族 → capability → Requirement → Scenario）
- 有顺序关系（delta 生成 → 守恒验证 → 归档）
- 按 §12 补档范畴，只新增文件、不改原制品

## 核心逻辑（伪代码）

### 关键数据结构

```
Marker       = { kind: 'full' | 'none', reason: string, line: int }
CheckResult  = { ok: bool, code: 'missing' | 'bad-kind' | 'empty-reason'
                       | 'no-file' | 'no-block' | 'legacy', hint: string }

parseMarker(proposalText) -> Marker | null
  // 在文件头部的 HTML 注释区内按行扫：
  //   <!-- diagrams: <kind>(<reason>) -->
  // 只认头部（首个非注释非空行之前），避免正文里的示例被误认

checkDiagrams(changeDir) -> CheckResult
```

### 主流程

```
checkDiagrams(changeDir):
    if not exists(changeDir/.openspec.yaml):          # legacy / 已归档补档
        return { ok: true, code: 'legacy' }

    m = parseMarker(read(changeDir/proposal.md))
    if m is null:            return red('missing',      '写 <!-- diagrams: full(理由) --> 或 none(理由)')
    if m.kind not in {full, none}: return red('bad-kind', '只认 full / none 两种')
    if m.reason.trim() == '':      return red('empty-reason', '括号里写一句大白话理由')

    if m.kind == 'none':
        return ok()                                    # 理由正确性不判

    f = changeDir/design-diagram.md
    if not exists(f):        return red('no-file',  'full 档需要 design-diagram.md')
    blocks = extractFencedBlocks(read(f), lang='mermaid')
    if len(blocks) == 0:     return red('no-block', '文件里至少放一块 ```mermaid')
    return ok()
```

### 失败与边界分支

- **marker 在正文而非头部**：不算。头部的定义是「首个非注释、非空行之前」——防止有人在正文里举反例（比如本 change 自己的 proposal 里就有示例）。
- **`none` 理由写得敷衍**（「改动较小」）：**不拦**。理由正确性不是机器的活（design D3）。若日后大量敷衍，那说明 review 没在看，加门禁解决不了。
- **已归档 change 补档**：有 `.openspec.yaml` 但目录在 `archive/` 下 → 视为 legacy 放行，否则补档老 change 会突然变红。
- **`full` 但图全画在 `design.md` 里**：判红。图的固定位置是 `design-diagram.md`，不然门禁要扫所有制品，且读的人不知道去哪找。
- **门禁脚本自身报错**（文件读不到、正则异常）：**判红并打出原因**，不静默通过——静默通过比漏检更坏，它会让人以为查过了。
