# test-cases: unify-lane-workbench

> complex 档交付账本。当前为用例设计，**未执行实现测试或浏览器验收**；下文期望不是实测结果。契约收口见 design Open Questions；阻塞项不得凭建议当作已批准需求。P1/P2 已完成，不重排前置任务。

## 测试落点索引

主链路表中的短码均展开为本表的仓库根相对路径；任务验证节另列机器可读的全路径映射。RN 是计划新增测试，不冒充现存资产。

| 短码 | 测试文件 / 人工留痕 |
| --- | --- |
| CO | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| CM | `front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts` |
| CC | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| NC | `front/app/features/tags/components/compose/LaneComposeCanvas.test.ts` |
| LM | `front/app/features/tags/components/console/LaneMemberList.test.ts` |
| AS | `front/app/features/tags/components/console/LaneAnchorScope.test.ts` |
| LC | `front/app/features/tags/composables/useLaneCorrection.test.ts` |
| LN | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| PL | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| API | `front/app/api/persistentTopics.test.ts` |
| RC | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` |
| TS | `front/app/features/tags/components/console/laneTrendSeries.test.ts` |
| RN（新增） | `front/app/features/tags/components/console/LaneRecommendationList.test.ts`（tasks 9.4 创建） |
| RP | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| CH | `backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go` |
| AH | `backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go` |
| AR | `backend-go/internal/topicgraph/repository/daily_report_assignment_test.go` |
| DH | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| BH | `backend-go/internal/tagmanagement/handler/board_crud_handler_test.go` |
| CL | `backend-go/internal/topicgraph/repository/compose_chain_test.go` |
| TM | `front/app/components/dialog/TopicManageDialog.test.ts` |
| AB | 人工：用 agent-browser 执行本文件 AB-1..AB-4，机械断言与截图在 change 的 `ui-verification/report.md` 建账，不沉淀业务 Playwright spec |

## 故事 S1：编辑既有泳道的成员与定义面

锚 Requirement：`lane-correction-surface / 泳道修正工作面是修正操作的主战场`；`workshop-lane-compose / 编排态入口与全屏画布`。

### 主链路（① 节拍串联）

最小 fixture：目标 active 泳道有窗口成员 4 个、更早成员 8 个；候选 A 无归属、B 属 candidate、C 属另一 active；A 是**唯一阈值内推荐**。原关系 A—D—E，D 对 A 一跳、E 仅两跳；另有仅因质心命中的 F。人工首义可配置含两个窗口外成员，另备自动首义场景。使用确定向量，不调 AI。

| 步 | 动作 | 来源 Scenario（delta 标题） | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| S1.1 | 矩阵行头「⋯」→ 编辑泳道 | 从矩阵行进入修正工作面；进入修正模式 | 左成员/首义、中央画布、右推荐均可见；左不含推荐；入口不顺带选中矩阵行 | 组件 + 浏览器 | CM；AB-1 |
| S1.2 | 分别检查左右清单 | 三栏内容不重复 | 左仅已选侧四块，右仅关联推荐；同一候选不在左右重复出现 | 组件 + 浏览器 | CM、LM、RN；AB-1 |
| S1.3 | 点击成员节点、键盘 Enter 聚焦 | 查看与选入分离（两能力同名） | 只 focus、卡跟随；成员仍 4/12，归属 API 0 次；不把卡上显式按钮算进画布本体点击 | 组件 | CC、NC、CM |
| S1.4 | 聚焦 A，再取消焦点 | 相关节点自动高亮 | D 与 F 高亮，E 不因两跳亮；其它弱化；取消后不残留旧高亮；一跳中的 active 仍只读 | 纯函数 + 组件 | PL、CC、NC |
| S1.5 | 卡上「＋加入本泳道」 | 节点卡快捷加入与移除；编辑泳道节点卡可快捷加入与移除 | 只 PATCH 一次；成功变成员/固定半径/勾号，计数 5/13；唯一推荐 A 消失、推荐数 −1；卡变移出动作，toast 表达加入 | 组件 + composable + 浏览器 | CC、CM、LC；AB-1 |
| S1.6 | 卡上移出，再用左栏移出另一个成员 | 节点卡快捷加入与移除；取消勾选即移除；取消勾选为破坏性动作 | 每次直接 topic_id:null，不选去处、不弹二次确认；对应节点划除、计数递减，进入本次已移出；与 ×取消聚焦不同样式 | 组件 + composable | CC、LM、CM、LC；AB-1 |
| S1.7 | 已移出行「加回来」 | 本次移出的节点可加回 | 同一 attach 路径；节点回成员、已移出项消失、计数 +1、toast「已加回」；写失败保留该项 | 组件 + composable | LM、LC、CM；AB-1 |
| S1.8 | 查看来源与尝试加入 A/B/C | 候选口径含动态泳道节点 | A/B 可选，B 标来源泳道名；C 无加入入口；归属状态未知时不假定可写；不改变其它话题状态/命中数 | 组件 + composable；既有 HTTP 回归 | LC、RN、CC；AH |
| S1.9 | 核对窗口徽标、展开更早成员 | 更早成员分组与计数口径 | 「窗口 4 · 全部 12」；更早 8 个按月分堆，能读日期/标题/快照接近度；两个面板的全部 M 相同 | 组件 + composable | LM、AS、LC |
| S1.10 | 查看首义选择池及当前/更早分区 | 首义选择池边界；首义面板当前/更早分区 | 池含全部有效成员 12 个，不含未加入候选/已移出；窗口内展开、更早折叠，标记现有首义；不要求切窗口才能查看更早 | 组件 + composable | AS、LC |
| S1.11 | 勾两个节点，覆盖人工首义 | 首义范围手动勾选 | 左栏发起、画布内确认条；确认后来源为人工 2，首义虚线同步；无 AppDialog，不请求立即重算 | 组件 + composable + 浏览器 | AS、CC、LC；AB-1 |
| S1.12 | 换为自动首义场景 | 首义来源可辨；自动首义不虚构节点列表 | 只显示自动来源，不伪造选定集/更早折叠带 | 组件 | AS、CC |
| S1.13 | 人工首义包含 2 个窗口外节点 | 首义节点在窗口外时左缘折叠带展示 | 左缘非比例折叠带含 2 点，点击出卡/可勾入重设；超过 6 点显示 +M，左栏可达全量 | 组件 + 浏览器 | CC、AS；AB-3 |
| S1.14 | 长清单滚到底部 | 左栏超高时操作按钮仍可达 | 两侧可各自滚动；首义提交按钮矩形落在栏可见区内，未靠裁剪腾空间；展开更早/错误提示在场时也测 | 组件 + 浏览器 | CM、AS；AB-3 |
| S1.15 | 标题点击→改名 Enter；再 Esc/失焦 | 画布标题就地改名；改名的次要路径不弹窗 | Enter 调既有更新一次，toast「泳道名已更新」；Esc/失焦无写且不退出工作面，失败回旧名并报错 | 组件 + composable | CM、LC |
| S1.16 | 切 <768px 并尝试入口 | 窄屏禁用画布；窄屏禁用编排 | 入口禁用并有原因；强行打开仅提示，不渲染半截三栏 | 组件 + 浏览器 | CM、CO；AB-3 |
| S1.17 | 完成一次写后返回矩阵 | 取消（编辑模式部分存在 OQ-3） | 按已批准 UI，返回不撤销已提交事实；矩阵 query 原样。delta「不保存任何改动」与此冲突，**待主线程收口后才判通过** | 组件 + 浏览器 | CM；AB-1；该节拍阻塞留痕 |

计数 N/M 的单位是 **section 成员数**，不是曲线线索数；一个成员内有 3 条非综合稿，不得让成员徽标 +3。生产推荐会随质心变化重新排序，只有本最小 fixture 可精确断言总条数 −1；一般场景断言被加入节点不再处于推荐组。

## 故事 S2：新建泳道仍是草稿，视觉/数值与编辑一致

锚 Requirement：`workshop-lane-compose / 编排态入口与全屏画布`；未改的「候选话题侧栏 / 成员质心与关联推荐 / 右栏职责与画布标题 / 保存与只读边界」作为回归网。OQ-4 未收口前不把新建改成无保存即时 PATCH。

### 主链路（① 节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| S2.1 | 从既有入口进新建 | 进入编排态 | 全屏三栏；原候选链/手动列表/检视能力仍可达，不新增既有 topic 的归属 PATCH | 组件 + 浏览器 | CO、LN；AB-2 |
| S2.2 | 带入候选链、手动选入，再单点节点 | 查看与选入分离；采纳预填（主 spec） | 草稿集合与选中态分开；点击只查看，显式卡片选入才改草稿，仍未写归属 | 组件 + composable | CO、LN、NC |
| S2.3 | 看同一已知距离样本的两处数值 | 两工作面接近度方向一致 | 内部 distance=0.2，两工作面展示接近度 0.80（各自按快照/估算标来源）；不再在新建卡片/检视栏显示距离 0.200；不强求不同来源数字相等 | 纯函数 + 组件 | PL、CO、CM、NC |
| S2.4 | 聚焦、更换焦点、取消聚焦 | 相关节点自动高亮（复用到新建的回归）；选中后相关节点显示标题缩略（主 spec） | 焦点相关集与已选标题集不混淆；无焦点恢复默认；原标题避让/active 只读提示保留 | 纯函数 + 组件 | PL、NC、CO |
| S2.5 | 在新建草稿后进入编辑，再返回新建 | 新建模式草稿不被修正模式污染 | 名称/已选 ID 保留，不被 correction load 清空或覆盖；两模式本地范围也不互写 | 组件 + composable | CO、CM、LN、LC |
| S2.6 | 改日期，取消确认；再确认并让请求失败 | 有草稿切换日期（主 spec） | 取消不取新候选；失败保留旧范围/草稿；仅确认且加载成功才清草稿；preview 不确认、不请求 | composable + 组件 | LN、CO |
| S2.7 | 空名保存→输入名称→保存 | 缺名称；保存成功（主 spec） | 空白拒绝且保留成员；成功走 createManualLane，调用次数 1，之后刷新宿主；不改变生命周期门禁 | 组件 + composable + 浏览器 | CO、LN；AB-2 |
| S2.8 | 有草稿取消、窄屏打开 | 取消；窄屏禁用编排 | 新建取消不落库；窄屏不渲染画布。新建出口与 UI 泛称「无保存」冲突部分待 OQ-4 收口 | 组件 | CO、LN |

## 故事 S3：泳道顶部时间视窗与曲线来源

来源合同目前是 proposal「顶部时间视窗」与 ui-design「曲线口径 / Data Provenance」，并非已补齐的 delta Requirement；**OQ-5 是来源 spec 缺口，主线程补锚前不宣称曲线验收通过**。已有主 spec `workshop-board-console / 泳道热度口径（线索数单一口径）`只提供线索量纲，不提供新增可选池选择集。

| 步 | 动作 | 来源 Scenario / 契约 | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| S3.1 | 用深链打开矩阵再开泳道 | ui-design Data Provenance；OQ-5 | 记住原 query，模态用自己的范围；打开/预览/提交/失败/关闭均不改 days/start/end | 组件 + 浏览器 | CO、CM；AB-1 |
| S3.2 | 编辑看曲线 | ui-design「编辑泳道」；OQ-5 | 只取目标 topic 的逐日 clue_count；其它泳道增减不改变该曲线；失败保旧可单独重试；目标未返回为未知 | composable + 纯函数 | LC、PL；效果核对 EV-1 |
| S3.3 | 新增看可选池 | ui-design「新增泳道」；OQ-2/OQ-5 | 非 active 有效 section 的非综合稿数逐日合计；无归属计入，active 排除；勾选不减池总量；不数 section/文章；90 天以外不冒充已知 | repository + API + composable | RP、CH、API、LN、PL；EV-2 |
| S3.4 | 拖动、松手、键盘、平移/缩放 | ui-design M5、复用控件合同；OQ-1 | preview 0 请求；一次 commit 1 请求；Arrow 逐日/Shift 逐周；pointercancel/Esc 0 提交；范围不超过 90 天、不越取数下界 | 纯函数 + 组件 | PL、RC、CO、CM |
| S3.5 | 7/14/30/365，新增再选 365/自选 | ui-design 档位合同（OQ-2 裁决放宽到 365）； | 泳道四档均可用；候选/画布/推荐同一实际窗口；失败不把按钮激活态留在未生效范围；矩阵原档位不变 | 组件 + composable | LN、LC、CO、CM |
| S3.6 | 当期无成员但有更早成员 | 更早成员分组与计数口径；主 spec `persistent-topic` | 窗口 0、全部 M>0，不说整条泳道无成员；推荐质心仍含有效更早成员，画布按窗口显示 | composable + 组件 | LC、LM、AS |

## ⓪ 改契约了吗：资产反查与继承调整

2026-11-16 已实际执行，均退出 0：

```bash
bash scripts/harness/test-assets.sh workshop-lane-compose
bash scripts/harness/test-assets.sh lane-correction-surface
```

反查结果：workshop 主 spec 8 Requirements / 40 Scenarios，archive 来源 `2026-11-02-rework-workshop-into-radar-and-desk`、`2026-11-11-lane-compose-story-chain`、`2026-11-16-lane-manual-correction-loop`；correction 主 spec 5 Requirements / 24 Scenarios，来源最后一项。历史有 test-cases，不代表映射已讲完 UI 故事：例如「进入修正模式」曾映射 DH，「新建草稿不被污染」曾映射 CC，均须补到宿主组件层，不能只照抄文件名。

### 旧断言冲突清单（已读测试源码）

| 旧资产 | 已存在的旧断言 | 本次处置 |
| --- | --- | --- |
| CM 的「修正模式两栏」 | `.bco__right` 不存在；左栏承担全部操作 | 改三栏可见/右300/左380；保留新建既有三栏与改名零弹窗，不用删 describe 逃避 |
| LM 的「关联推荐」三例 | 左栏推荐行 add、接近度、推荐空态 | 迁 RN；LM 移除 recommendations fixture/prop，反向断言无推荐分组 |
| LM 的「接近度两把尺子」 | 同一左栏内比较成员快照与推荐估算 | 成员快照半边保留 LM；估算半边迁 RN；CM 增跨左右同屏可辨来源断言 |
| CC 的「图例只4个符号」 | 图例串为「成员大小=接近度虚线=首义⊘已移出」 | 改为「候选大小=接近度」，成员固定尺寸；相关/只读/取消聚焦信息按新合同核对，不保留固定四项限制 |
| CC 的点节点测试 | 点击节点不 emit add/remove | **继承**，不是冲突；另测点卡片按钮才 emit，禁止把整条负向断言删掉 |
| 新建卡片/检视实现 | 实际模板仍展示「距离」；既有测试未锚该展示方向 | 在 CO/NC 补「接近度」挂载断言与 distance=0/0.2 样本；不能只改纯函数、让模板继续反向 |
| NC 既有相关标题 | 已选集的一跳与质心推荐用于标题；Z7 缩放不影响标题 | 保留标题用例，新增焦点高亮用例；不让焦点新功能替换已选标题故事 |

### 继承与调整：workshop-lane-compose（40 行）

「旧测试」列使用索引短码；历史无直接 UI 映射者显式列出旧资产及补点，不把后端文件当组件证据。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 进入编排态 | 继承 | CO | 保留入口/全屏；时间头接线后重跑 |
| 查看与选入分离 | 改语义 | CO；历史另有人工记录 | 保留本体点击不选入，增加卡片按钮是显式动作 |
| 进入修正模式 | 改语义 | DH；实际补查 CM | 从两栏改三栏，映射新增 CM，DH 不充当三栏证据 |
| 取消勾选为破坏性动作 | 继承 | AH；补查 LM | 解除归属语义不变，补 CC/CM 的卡片入口与视觉区别 |
| 新建模式草稿不被修正模式污染 | 继承 | 历史映射 CC | 补 CO/CM 宿主切模式故事，CC 单独不能证明草稿保留 |
| 窄屏禁用编排 | 继承 | 历史人工；补查 CO/CM | 两模式入口/强行挂载都断言无残缺工作台 |
| 取消 | 改语义待收口 | CO；历史人工 | 新建弃草稿继承；编辑已即时落库的返回不得撤销，OQ-3 阻塞最终判定 |
| 快捷日期与边界 | 改 UI 载体 | CO、DH | 保留含首末/锚点日，增加上限 365 的回归；新控件接线见 6.1/6.4 |Q-1 |
| 有草稿切换日期 | 继承 | CO | LN 补 preview不清草稿、取消与失败回滚 |
| 倒置或越界日期 | 继承 | DH | CH/LN 补1/365/366日与下界，不因换控件变静默修正 |
| 跨期连接而非连续出现 | 继承 | CL | 不改候选链算法，保留服务端来源 |
| 没连接或跨度不足 | 继承 | CL | 不因高亮推断新边或制造候选链 |
| 采纳预填 | 继承 | LN、CO | 保留整组带入与自动名 |
| 取消整组与半选 | 继承 | LN、CO | 保留原生三态与成员计数 |
| 人工名称不被覆盖 | 继承 | LN | 时间窗失败/取消不吞人工名 |
| 清空与取消本次 | 继承 | LN | 新建清空/取消仍只影响草稿 |
| 已有泳道显式参考 | 继承 | CO、LN | active 仍显式只读，不因相关高亮变可选 |
| 历史启用资格 | 继承 | CO、LN | 名单/带入/启用入口保留，不放宽 can_activate |
| 从手动列表选事件 | 继承 | CO | 左侧旧手动列表能力不得被新增空态原型误退役，OQ-4 收口 |
| 画布节点可识别 | 改表达 | NC | 用共享成员固定/候选映射/只读弱化规则；文字可达 |
| 搜索过滤 | 继承 | CO | 名称过滤不改变真实已选集 |
| 归档来源可识别 | 继承 | CO | 只标来源，不复活归档话题 |
| 采纳后质量刷新 | 继承 | PL、LN | 每次草稿增删重算有效成员均值 |
| 点击节点推荐关联 | 继承并补节拍 | PL、LN | 焦点不成为质心；补焦点相关 union/取消恢复 |
| 从推荐项加入 | 继承 | PL、LN | 选入是草稿，不误接 correction PATCH |
| 无关联 | 继承 | PL | 无一跳且无推荐时不造高亮/关系 |
| 无距离数据 | 继承 | PL | null 不变0、不展示假接近度 |
| 相近程度不等于已选 | 改表达 | PL | 成员固定且实心勾，候选大小只编码接近度 |
| 右栏只做三件事 | 继承 | CO | 保留新建检视/已选/可补充能力；不把编辑右栏合同直接覆盖新建 |
| 已选里取消勾选 | 继承 | CO | 新建移出草稿与编辑解除归属分开 |
| 画布标题可读且不打架 | 继承 | NC | 共享半径后重测标签避让/边界 |
| 选中后相关节点显示标题缩略 | 继承 | PL；补 NC | 已选标题集保留，与焦点高亮不同输入同一实现 |
| 缩放/平移后节点卡不塌扁 | 继承 | NC | 保留固定卡宽、跟随与边界 |
| 拖拽平移不误伤点选 | 继承 | NC | 超阈值拖后 click 不聚焦、不写入 |
| 相关节点在已有泳道里 | 继承 | PL | 一跳可强调 active，但仍只读/不自动选入 |
| 节点卡跟随选中节点并可快捷选入 | 继承 | CO | 新建卡片仍改草稿，编辑卡片新增即时写路径 |
| 取消节点卡与取消选中 | 继承 | CO | Esc/×/空白只清焦点与高亮，不改已选 |
| 缺名称 | 继承 | CO | 空串/空白拒绝，已选不丢 |
| 保存成功 | 继承 | CO | 保留 createManualLane；OQ-4 不得偷删保存出口 |
| 窄屏提示 | 继承 | CO | <768px 提示，>=768px 不读取窄屏专有状态 |

### 继承与调整：lane-correction-surface（24 行）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 从矩阵行进入修正工作面 | 改语义 | DH；实际补查 CM | 两栏→三栏，用 CM/AB-1 证明，不只跑 handler |
| 查看与选入分离 | 改语义 | 历史人工；补查 CC | 保留本体只 focus，补卡片显式写按钮 |
| 取消勾选即移除 | 继承 | AH；补 LM/LC | 不选去处；新增卡片走同一 removeMember |
| 本次移出的节点可加回 | 继承 | AR；补 LM/LC | 加回同一状态，不加第二份成员缓存 |
| 首义范围手动勾选 | 继承 | AS | 左栏发起/画布确认，保留 pending/失败选集 |
| 首义来源可辨 | 继承 | CC | 自动/人工仍来源可辨 |
| 左栏超高时操作按钮仍可达 | 改布局后回归 | 历史人工 | AB-3量两侧滚动与sticky，追加 CM/AS 样式结构锚 |
| 首义节点在窗口外时左缘折叠带展示 | 继承 | CC | 不因节点表达共享丢折叠带/6点上限 |
| 自动首义不虚构节点列表 | 继承 | AS | 无人工集时无折叠带 |
| 首义选择池边界 | 继承 | AS | 仅当前成员；候选/已移出不入池 |
| 候选口径含动态泳道节点 | 迁移位置、继承资格 | LC | 关联推荐搬右栏、来源可见；OQ-3修旧「左栏」措辞 |
| 首义面板当前/更早分区 | 继承 | AS | 默认展开/折叠、全部 M 同源 |
| 更早成员分组与计数口径 | 继承 | LM、AS | 保留同向量过滤与P2总数，不用曲线clue_count替代 |
| 窄屏禁用画布 | 继承 | 历史人工；补 CM | 不渲染三栏且入口禁用 |
| 画布标题就地改名 | 继承 | 历史映射 CC；实际 CM | Enter/Esc/blur/失败落宿主测试 |
| 改名的次要路径不弹窗 | 继承 | 历史映射 CC；实际 CM | 保留全程零弹窗，不把首义确认改成弹窗 |
| 编辑版块弹窗 | 继承、不改实现 | BH | 与泳道三栏无关，旧入口保留；不为本change改版块表单 |
| 归档泳道可逆 | 继承、不改实现 | AH | 仍走治理入口，不搬入工作台 |
| 界面无解说性长句 | 继承并修旧图例 | 历史人工；补 CC | 改「候选大小=接近度」，保留禁教学/公式；AB-3文本审计 |
| 雷达行入口不误跳转 | 继承 | 历史人工 | AB-4点编辑不触发雷达行导航 |
| 矩阵行头入口不误选中 | 继承 | 历史人工 | AB-1点菜单不改矩阵当前格 |
| 泳道管理入口命名 | 继承 | TM | 保留「编辑泳道」，不退回多功能弹窗 |
| 无来源徽标不出现 | 继承 | 历史人工 | RN/CC挂载断言无装饰性来源/事件身份徽标 |
| 接近度口径统一 | 继承方向、迁移位置 | 历史人工；实际 LM/LC | 快照留左、估算迁右，补 CO/CM 两模式展示；来源不同不强求数值相同 |

无 Scenario 整项废止；废止的是「编辑两栏/无右栏」与「推荐在左栏」的旧布局断言，不是取消旧能力。64 行逐一处置，不能用一条「回归通过」替代。

## ② 变体走查（五组固定清单）

| # | 变体（组/固定条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| I1 | 输入：空串 | 名称空串拒绝且保留成员；日期缺一边拒绝、不请求 | 组件/composable/HTTP | CO、CM、LN、CH |
| I2 | 输入：纯空白（全角空格/tab） | 名称trim后空拒绝；日期不是有效YYYY-MM-DD时行内报错，旧范围/输入草稿可恢复 | 组件/composable | CO、CM、LN |
| I3 | ~~输入：纯分隔符（单/连续/首/尾）~~ | 不适用：本change没有多token解析器；名称内分隔符当普通文本，日期依固定格式拒绝 | 留痕 | 不另造解析测试 |
| I4 | 输入：单token | 一个合法名称可保存；单个section有向量可作为成员；无有效质心不伪造推荐 | composable/纯函数 | LN、PL |
| I5 | ~~输入：大小写变体~~ | 不适用：ID/日期/向量不分大小写；不改已有名称搜索匹配策略 | 留痕 | CO 原搜索回归保留 |
| I6 | 输入：正则元字符/引号/emoji | 名称/标题按文本展示，不当HTML/正则执行；日期字段拒绝这些字符 | 组件 | CO、CM、RN |
| I7 | 输入：超长输入 | 长标题省略号+title可取全文，按钮不被挤走；沿用名称既有服务端限长，不新增臆造上限 | 组件/浏览器 | RN、CM；AB-3 |
| P1 | 前置：空集 | 新增无池节点=空态；编辑无成员与仅本窗无成员分别呈现；无 completed 日报无伪造今天 | 组件/composable | LN、LC、CM、CH |
| P2 | 前置：单元素 | 卡片/半径/计数可用；一条section多条线索仍只算一个成员 | 纯函数/组件 | PL、LC、CO |
| P3 | 前置：重复元素 | 按section ID去重计池/高亮；一次加入不会成员+2；不重复同一原关系 | 纯函数/composable | PL、LC、LN |
| P4 | 前置：越界/跨域引用 | 无效board/他板topic延用现有4xx；active其它泳道节点不可点加入；未知归属不默认可写 | 组件/既有HTTP | CC、RN、CH、AH |
| P5 | 前置：部分满足 | 部分成员无向量不进质心；有效更早成员仍参与；池中有缺失clueCount/未知归属的当日显示未知 | 纯函数/composable/API | PL、LC、LN、API |
| T1 | 时间窗口：边界两端、前后各一步 | 闭区间含最新completed日；latest−364可用、−365拒绝；跨度1/365可用、91拒绝；不以系统今天替锚点 | HTTP/纯函数 | CH、PL、LN、LC |
| T2 | 时间窗口：空窗口 | 请求成功且区间内无可选section，池可选总量为已知0；请求失败/未加载不当0；无报日锚点控件禁用 | 组件/composable | LN、LC、CO、CM |
| T3 | 时间窗口：跨窗口 | 首义/成员由窗口内转更早、再转回，全集M不变；窗口变更不改变已落库归属；新建确认/失败不丢旧草稿 | composable/组件 | LC、AS、LM、LN |
| T4 | 时间窗口：时区/日期归一化 | 服务端日期取YYYY-MM-DD，以UTC日算术验证；本地Asia/Shanghai显示不偏一天；月末/闰日合法，非法2/30拒绝 | HTTP/纯函数 | CH、PL、LN |
| J1 | 幂等：重复执行两次 | pending时同节点第二次点击不再请求；相同已提交范围不再取候选；focus/clear不写库 | 组件/composable | CC、RN、CM、LC、LN |
| J2 | 幂等：部分失败后重试 | 写失败集合/卡片保留；曲线失败旧曲线标未刷新，单独重试；候选失败旧范围保留；成功仅更新一次 | 组件/composable | LC、LN、CM |
| J3 | ~~幂等：跨线程并发安全~~ | 不适用：本change不声称多客户端写入/缓存线程安全；但模态内部迟到响应隔离按B9测，不划掉异步串台用例 | 留痕 + composable | LC、LN |
| A1 | 可用性：误输入反馈 | 非法日期/空名给字段旁错误，不清成员；手输非法不静默钳制，手势越界才按限界处理 | 组件/composable | CO、CM、LN、LC |
| A2 | 可用性：空态 | 新增「无可编排内容」、编辑无成员、推荐无新增、当窗无节点各有状态；不可用数据不能写成无内容 | 组件 | CO、CM、CC、RN |
| A3 | 可用性：错误态 | candidates/anchor/curve分别报错可重试；写4xx不显示成功，不刷新整页；其它active仍只读 | 组件/composable | LC、LN、CM、RN |
| A4 | 可用性：加载态 | 骨架不画假节点；曲线加载不抹除左栏；pending同步禁用卡片和行，不整页锁死 | 组件 | CC、CM、RN |
| A5 | 可用性：超长文本 | 两主题/双桌面档下title全文可达，省略号非硬裁，长单词不撑破三栏；各交互在场态测 | 浏览器 | AB-3 |
| A6 | 可用性：重复提交 | 双击加入/移出、Enter连按改名/提交首义，同一动作pending只一次请求；结束后可操作其它节点 | 组件/composable | CM、LC、RN、CC |

## ③ 层选择与浏览器故事落点

- PL 测纯运算，不碰 DB：半径/强弱、1跳union、接近度换算、逐日聚合与未知传播、日期限界。
- RP 使用 `testutil.SetupTestDB` 的隔离 pgvector testcontainer；无 SQLite、无开发/生产 DSN、无 t.Parallel。CH 负责 HTTP 字段/90日参数回归，不要求改既有 handler 窗口策略。
- 组件层必须真实挂载卡片/按钮，断言 active/pending 的可点性、错误文案与点击后状态；不只 mock 掉事件检查函数调用。happy-dom 不断言 `color-mix()` inline 字符串，断 class/data属性，颜色/矩形交给浏览器。
- **AB-1 是不可被组件用例替代的完整主故事**；不是「人工豁免」。使用 agent-browser 按需执行，保存机械断言结果/截图，不新增业务 Playwright 脚本。写链路只使用主线程明确批准可修改的验收泳道或隔离数据；若无授权数据，标未验证，不在真库造fixture/清理数据。

### AB-1：矩阵→编辑→聚焦→加入→移出→加回→时间窗→返回

静态 `http://localhost:5100` 同源发布；先用只读查询确定 BOARD_ID/TOPIC_ID 和最小推荐fixture，关闭引导遮罩。通过 `agent-browser --session lane-workbench open "http://localhost:5100/tags/$BOARD_ID"` 进入（变量须先设为验收对象）。

1. 保存 location.href、矩阵选中格和初始成员/候选ID；点行头编辑，断三栏可见、左无推荐/右无成员、SVG宽等容器宽。
2. 点候选A：归属请求0次、成员数不变；节点卡贴近节点，A的一跳/质心推荐命中集合与PL fixture一致；取消恢复默认。
3. 再聚焦A→点卡加入：断请求成功、A转成员、左N/M各+1、A退出右推荐；在唯一推荐fixture下计数−1；右行/左勾选/卡片一致。
4. 卡上移出：断解除归属、N/M各−1、划除和已移出行出现；无去处选择器/二次确认。点「加回来」断反向变化。
5. 修改时间范围：拖动只preview，松手一次提交；档位/表单/曲线选区/三栏实际窗口一致；URL的days/start/end逐字不变；非法范围保旧。
6. 返回后矩阵仍在原深链、能看到已提交事实。再次只读核对归属。OQ-3未收口则该项记阻塞，不把冲突解释成通过。

### AB-2：新建草稿→选入→比较方向→日期取消/重试→命名保存

1. 从既有入口开新建，带入候选链、手动勾一个节点，检查卡片只改草稿，无assignment PATCH；查距离0.2样本显示接近度0.80/估算。
2. 有草稿改范围取消，名称/成员保留；制造一次受控请求失败再重试，失败保旧、成功才切换。切编辑再返回，新建草稿仍在。
3. 空名拒绝且保留选择；合法名称提交既有createManualLane一次，确认创建active/manual后由宿主返回。只使用已批准测试数据。OQ-4未明确出口/栏位前该段不得验收。

### AB-3：可达性、四态与布局

- 1440×900、1920×1080，editorial/dark 各检查编辑中与空态，截图至少8张；新建空态和已确认态另对批准三态图留证。375×667、390×844检查窄屏禁用；767px/768px边界组件用例补齐。
- 机械断言：侧栏宽380/300px；中心宽>0、SVG/clientWidth 1:1；body无横向溢出；长标题/长单词/更早分组展开/错误提示/首义确认/节点卡在场态逐个测量。滚动后sticky按钮在可视区且 `elementFromPoint` 命中按钮/工作面，卡不被遮住。
- 画布缩放0.5×/1×/3×后卡仍可点，拖超过阈值不误触；焦点环、键盘Enter、Esc取消层级；`prefers-reduced-motion: reduce` 无透明度过渡。
- 图例为候选大小编码接近度，成员大小不编码；不含教程长句/质心公式/事件线身份数；完整标题经title/详情可达。

### AB-4：入口与旧能力去向回归

从雷达编辑、矩阵行头编辑、管理列表编辑各走一遍，检查行事件未误冒泡。按布局规范补全系统路由矩阵（`/`、`/search`、`/discovery`、`/settings`、`/tags`、`/tags/:boardId`、`/tags/:boardId/:date`、`/tags/:boardId/research`，两主题桌面；本次触窄屏的路由另检查窄屏），区分「未覆盖」与pass。至少走「矩阵编辑后回原深链」「新建候选链→命名保存」「无窗口成员→展开更早→重设首义」三条产品任务。旧能力去向表列候选链、手动列表、历史带入/启用、改名、首义、移出/加回；未迁移也未批准退役的记fail。

证据格式：`ui-verification/report.md` 的 `### A-N <场景>【pass|fail|未覆盖】` + 依据行、截图相对路径、视口/主题/fixture标识；未执行不写pass。

## ④ 效果核对（EV，只读真库）

### 触发原因与安全边界

曲线依赖真实scope/归属/有效向量分布。fixture绿不能证明可选池没有漏无归属或混入综合稿，因此实施后必须只读核对。**本轮不执行真库核对；真库只准SELECT，禁DDL/DML、建临时表、回填、造数或修数据。** psql客户端额外设只读；写链路fixture不从本节准备。

先人工选一块有数据的board和一个泳道（`BOARD_ID`/`TOPIC_ID`，不得拿例子ID当真值），以下命令只输出365天section审计底表：

```bash
test -n "${BOARD_ID:-}" && test -n "${TOPIC_ID:-}" && \
docker compose -f docker-compose.pg.yml exec -T \
  -e PGOPTIONS='-c default_transaction_read_only=on' postgres \
  psql -X -v ON_ERROR_STOP=1 -U postgres -d syntopica \
  -v board_id="$BOARD_ID" -v topic_id="$TOPIC_ID" <<'SQL'
WITH latest AS (
  SELECT MAX(period_date) AS day
  FROM board_daily_reports
  WHERE semantic_board_id = :'board_id'::bigint AND status = 'completed'
)
SELECT r.period_date, s.id AS section_id, s.persistent_topic_id,
       p.status AS owner_status, (s.embedding IS NOT NULL) AS usable_vector,
       COUNT(t.id) AS thread_count,
       COUNT(t.id) FILTER (WHERE COALESCE(t.scope, '') <> 'composite') AS clue_count,
       COUNT(t.id) FILTER (WHERE t.scope = 'composite') AS composite_count,
       (r.period_date >= latest.day - 364) AS in_compose_pool,
       (s.persistent_topic_id = :'topic_id'::bigint) AS target_lane
FROM daily_report_sections s
JOIN board_daily_reports r ON r.id = s.report_id
CROSS JOIN latest
LEFT JOIN board_persistent_topics p ON p.id = s.persistent_topic_id
LEFT JOIN daily_report_threads t ON t.section_id = s.id
WHERE r.semantic_board_id = :'board_id'::bigint AND r.status = 'completed'
  AND r.period_date BETWEEN latest.day - 364 AND latest.day
GROUP BY r.period_date, s.id, s.persistent_topic_id, p.status,
         (s.embedding IS NOT NULL), latest.day
ORDER BY r.period_date, s.id;
SQL
```

配对GET（只读）：

```bash
curl --noproxy '*' -fsS "http://localhost:5100/api/semantic-boards/$BOARD_ID/lane-dynamics?days=365"
curl --noproxy '*' -fsS "http://localhost:5100/api/semantic-boards/$BOARD_ID/persistent-topics/compose-candidates?days=90"
```

### EV-1 编辑泳道与 EV-2 新增可选池

| 核对 | 方法 | 量化结果要求 | 结论填写 |
| --- | --- | --- | --- |
| EV-1 编辑逐日线索 | SQL中target_lane=true的section按日合计（不加向量过滤，沿lane-dynamics口径）对比GET目标泳道每天clue_count与UI；用API真实存在的日格对账，缺泳道不补0 | 已返回日格差异数=0，综合稿混入数=0；记录目标topic与日期数 | 待执行：实测后填样本数/差异数；未取到标未知，不当达标 |
| EV-2 每section计数 | GET每条section与SQL同ID对clue_count，包含0与综合稿混合样本 | 字段存在率100%，计数差异数=0；0不省略 | 待执行 |
| EV-2 可选池逐日合计 | SQL筛in_compose_pool、usable_vector，纳入无归属及已知非active来源，排active；与GET同一section ID集合逐日聚合对比UI；归属brief缺失单独记未知，不假定可选 | 可选ID集合差异=0，逐日差异=0；分别记录无归属/非active/active排除数；草稿勾选前后曲线变化=0 | 待执行；若无某种样本，明确未覆盖而非0差异即全覆盖 |
| 未知与来源覆盖 | 数旧响应缺字段、归属brief缺失、lane未返回、窗口外不可得的天数 | 记录未知日数及原因；未知显示成0的次数=0 | 待执行；OQ-2未收口不声称新增365曲线已核对 |

不达标先定位上游缺向量/缺归属brief或本次聚合错误，量化交主线程；不为让图好看而写库修scope/补计数。

## ⑤ 展示字段盘点（来源语义锚）

| 用户可见字段 | 来源与语义 | 已有 spec 锚 | 本次动作 / 缺口 |
| --- | --- | --- | --- |
| 成员接近度/快照标注 | `1−topic_match_distance`；0距离=1.00；缺距离无数字 | 主 `persistent-topic / 泳道成员计数与接近度的单一口径` | 继承；LM/CM/LC对照，不能因迁移统一成估算 |
| 右栏接近度（估算） | `1−cosineFor(有效当前成员均值,候选embedding)`；cosineFor返回距离，不是cos相似值 | 同上；delta `两工作面接近度方向一致`；correction主Requirement的候选接近度句 | 估算标注与来源名迁RN；PL/CO/CM断方向；ui-design的1−cos文字交OQ-4修订 |
| 窗口N/全部M、更早K、首义池徽标 | 同一有效成员section全集分层，M=N+K；不是线索数 | correction delta `更早成员分组与计数口径`、`首义面板当前/更早分区`；主persistent-topic同上 | 保留P2同定义/同过滤；LC/LM/AS |
| 右栏关联推荐·N | 当前未选、可加入且阈值内的推荐条目数，不含active只读；加入后重算 | correction delta `三栏内容不重复`、`节点卡快捷加入与移除` | 迁移计数与行同源；一般质心重算不承诺总数必−1 |
| 编辑曲线口径文字「本泳道·线索数」 | lane-dynamics中目标泳道每天非综合稿线索数 | 主`workshop-board-console / 泳道热度口径（线索数单一口径）`仅锚量纲 | **泳道工作面选取目标泳道及365参考/90可编排缺专属spec锚，OQ-5；不是以UI表替代spec** |
| 新增曲线文字「可选线索数（不含active）」及数值 | compose-candidates有效section的clue_count，排active、含无归属；缺项未知 | **当前delta无此来源Requirement** | OQ-5阻塞展示实现；D2补字段是数据前置，不能宣称spec已齐 |
| 日期范围/参考范围反馈 | 服务端window回显，模态本地，365参考不等于365可编排 | 主workshop「编排日期范围」仅覆盖已有候选窗口；模态不写query在UI合同 | OQ-1/OQ-2/OQ-5收口后补来源与范围spec锚；LN/LC/CO/CM |
| 首义自动/人工N、生效时点 | anchor API source/section_ids；归属立即、首义次日 | correction delta `首义来源可辨`、`首义范围手动勾选`；主`lane-manual-correction` | 继承，不从可见节点数伪造自动首义集 |

## 白盒附加（complex）

### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 / 落点 |
| --- | --- | --- | --- | --- |
| B1 | 身份优先级 | 成员、候选、只读、已移出，叠首义/焦点 | 成员固定尺寸，候选按接近度，只读弱小，移出划除；首义/焦点正交 | `memberAppearanceIgnoresDistance`、`independentNodeFlags` / PL、CC、NC |
| B2 | 相关并集 | focus=A，A-D-D-E，F仅推荐，推荐列表含A/重复F | 得D/F、不传递到E、不含A、不重复；不改所选 | `focusedRelatedUnionIsOneHop` / PL |
| B3 | 空焦点/无质心 | focus=null；或已有focus但成员向量全无 | 无焦点不弱化；无质心仍可一跳，不造推荐 | `clearFocusRestoresDefault` / PL、CC、NC |
| B4 | 两来源接近度 | snapshot距离/estimate距离/null/0 | 同向1−距离，来源标注不同，null隐藏，0保留 | `proximityDirectionAndProvenance` / PL、CO、CM |
| B5 | 权限与pending | candidate/无归属/active/归属未知，pending同ID | 前两者可加入，后两者不可；pending卡/行均禁用，重复调用只一次 | `cardAndRowSharePending` / CC、RN、CM、LC |
| B6 | 写成功/失败 | attach/detach/restore各success、4xx、reject | 成功集合变一次；失败保持集合/卡、无成功toast，可重试 | `cardAssignmentFailureRetainsFocus` / LC、CM |
| B7 | 曲线口径 | 2条single+1条composite；另一active高计数 | section clue=2；新增排active/含无归属，编辑只取目标泳道 | `TestGetComposeCandidates_ClueCountExcludesComposite` / RP；`workbenchSeriesUsesClues` / PL、LN、LC |
| B8 | 未知传播 | eligible某项缺clueCount/缺归属状态；0和缺字段混合 | 受影响日null；已知0不变null；不把部分和当总量 | `poolSeriesPropagatesUnknown` / PL、API、LN |
| B9 | 请求串台 | 旧窗口慢、新窗口快；关闭/切板后旧响应 | 只有当前请求能提交；旧响应不改范围/清单/曲线/query | `staleWorkbenchResponseIsDiscarded` / LC、LN |
| B10 | 日期提交 | preview/commit/cancel；非法手输；超界手势 | preview/cancel不请求；commit合法一次；非法保旧；手势限界不扩后端 | `workbenchPreviewAndCommitAreSeparate` / CO、CM、LN、LC |
| B11 | 更早成员质心 | 窗口成员0，更早成员有向量；更早也无向量 | 前者仍可推荐，后者仅不展示接近度、不伪造质心 | `earlierMembersStillSeedRecommendations` / LC |
| B12 | 旧后端 | early_members省略；clue_count省略；目标lane省略 | 全量徽标降级窗口数，曲线未知；不拿0或section数补位 | `legacyWorkbenchFieldsStayUnknown` / API、LC、LN |
| B13 | 新建选择集 | 已选/取消/窗口确认失败 | 选入仅草稿；失败不清名称/集合；不触发assignment PATCH | `createDraftNeverCallsCorrectionWrites` / CO、LN |
| B14 | 右栏迁移 | 有推荐/无推荐/错误/pending/长标题 | 仅右栏承接来源/估算/动作/四态，左无重复 | `recommendationsLiveOnlyOnRight` / CM、LM、RN |

### 边界值清单

| # | 变量 / 边界值 | 期望 | 落点 |
| --- | --- | --- | --- |
| E1 | 曲线全0 | 画已知零基线，不显示未取到，不制造正值刻度 | PL、TS、LN、LC |
| E2 | 曲线部分未知/全未知 | null处断段、不桥接不补0；全未知给状态而非空池结论 | PL、TS、API |
| E3 | 泳道无成员 | 总数0、无质心；只读/候选仍可展示可证明来源，不伪造首义 | LC、CC、CM |
| E4 | 窗口内0成员但更早M>0 | 显示窗口0/全部M，质心含有效更早成员，首义池不空 | LC、LM、AS |
| E5 | 新增可选池为空/只有active | 可选总量0、不能创建空成员泳道；active参照不计曲线 | LN、CO、PL |
| E6 | 1/7/14/30/90/364/365/366日 | 1与365闭区间可用；366表单拒绝；三处上限一致（后端常量/报错文案/前端校验），绝不依赖后端静默截断 | LN、LC、CH、CO、CM |
| E7 | 最新日报日−364 / −365、跨月/闰日 | −364含入、−365拒绝；日期不受本地时区偏移 | CH、PL、LN |
| E8 | distance=0、threshold、threshold+ε、null | 0→接近度1.00；阈值含等号，+ε不进推荐，null不显示 | PL、LC、CO、CM |
| E9 | 已选向量0/1/多条、零范数/不同维/NaN | 无有效质心不推荐；合法均值不取首个/焦点，非法样本不造距离 | PL |
| E10 | 折叠带0/1/6/7节点 | 无点不展示；6全显；7显示6与+1，全量左栏可达 | CC、AS |
| E11 | 767/768px、375×667、390×844 | <768禁入口/提示，>=768不读取窄屏专有状态；桌面三栏见E12 | CM、CO；AB-3 |
| E12 | 1440×900 / 1920×1080、两主题 | 380/300两侧、中心>0、无页面横溢出；长清单/卡/确认/错误在场亦成立 | AB-3 |
| E13 | 缩放0.5×/1×/3×、拖动阈值前后 | 普通点>=3px可见、卡可点；超阈值不误聚焦/选入，计数不变 | PL、NC、CC；AB-3 |
| E14 | 0 thread / 全composite / mixed | clue_count分别0/0/非composite条数，字段不省略；不是文章总数 | RP、CH、API |
| E15 | 同日多section、重复ID、选择后重算 | 曲线同日求和且ID去重；成员只按section计；选择不减固定可选池曲线 | PL、LN、LC |

### 不适用划除（留痕）

- ~~数据库迁移/回填分支~~：只加JSON投影，无DDL、无存量重建。
- ~~AI调用失败/质量分支~~：本change不调用AI；曲线计数、质心、高亮由现有事实/纯函数产生。
- ~~多选批量移出~~：ui-design明确不做；只测单节点动作与重复提交防护。
- ~~365天候选向量取数~~：不是遗漏测试，而是禁止的工作面档位；新增365背景仍为OQ-2，不能写一个虚构成功用例。
