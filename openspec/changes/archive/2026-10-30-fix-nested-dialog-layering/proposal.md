<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

# Proposal: fix-nested-dialog-layering

## Why

版块治理工作面（本板块上下文 / 治理，`AppDialog size="xl"`，遮罩层级 1000）里点「创建并挂载」打开的组合标签创建弹窗（`CompositeLabelEditDialog`，Teleport 到 body，遮罩层级在 CSS 里写死 **60**）**被宿主弹窗整块盖住**——用户点了按钮，界面毫无反应，弹窗完全不可见（2026-10-30 用户实机截图：工作面停在「组合」页签，创建弹窗看不见）。

根因不是「弹窗没打开」，而是**浮层层级没有契约**：宿主遮罩与嵌套浮层都是 `body` 的直接子节点，绘制顺序只由 z-index 决定，而嵌套浮层写死的 60 < 宿主弹窗 1000，于是嵌套浮层被宿主遮罩（连同宿主的 backdrop blur）整体盖住。

为什么今天才炸：`unified-dialog` spec 的约束长期把「嵌套 dialog」写成禁区——「不支持嵌套 dialog（如果业务需要，后续再加 z-index 管理）」。这次业务第一次真的在弹窗里再开弹窗（治理工作面 → 创建组合标签），而「z-index 管理」这一半从未做，于是新浮层只能随手写个数字（60）。同类隐患是结构性的：层级档位散落在各组件里（AppDialog 1000 / ArticlePreviewModal 9100 / 弹窗内全屏 9200 / toast 9999 / 各手写 overlay 30~200），**没有档位表可依**，下一个新浮层还会踩同一个坑。

## What Changes

- **浮层层级契约落到代码（单一事实源）**：`front/app/components/ui/layout-contract.ts` 新增 `Z_INDEX` 档位表（`dialog` 1000 / `dialogNested` 1100 / `readingDialog` 9100 / `readingFullscreen` 9200 / `toast` 9999）与 `nestedOverlayZ(hostZ)` 相对抬升 helper；`AppDialog` 默认 `zIndex` 改取 `Z_INDEX.dialog`（数值不变 1000，行为零变化，只换取值口）。
- **组合创建弹窗走嵌套档**：`CompositeLabelEditDialog` 新增 `zIndex` prop（默认 `Z_INDEX.dialogNested` = 1100），遮罩层级由 CSS 手写 60 改为内联绑定——在治理工作面（1000）内打开时层级严格高于宿主遮罩，不再被盖住；宿主若处更高档（阅读层 9100）可显式传值按 `nestedOverlayZ(hostZ)` 抬升。
- **层级契约写进权威文档**：`docs/reference/standard/frontend/layout.md` 增「浮层层级契约」节（档位表 + 「弹窗内浮层 MUST 高于宿主遮罩」规则 + 新写浮层必须取档位不得手写数字）；`unified-dialog` spec 补「浮层层级契约」Requirement，把原「不支持嵌套 dialog」约束升级为层级管理条款。
- **回归锚（防「改回手写数字」）**：三条机械断言落单测——① 档位表顺序单调（dialog < dialogNested < readingDialog < readingFullscreen < toast）；② 组合弹窗渲染层级严格大于宿主弹窗遮罩层级（组件层直接断言两个内联 z-index 的比较）；③ `nestedOverlayZ(hostZ)` 严格大于 hostZ。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `unified-dialog`: 新增「浮层层级契约」Requirement（档位表唯一事实源 + 嵌套浮层必须高于宿主遮罩 + 宿主可按 `nestedOverlayZ` 抬升 + `AppDialog` 默认层级取档位）；原「不支持嵌套 dialog（如果业务需要，后续再加 z-index 管理）」约束升级为层级管理条款。

## Impact

- **前端**：`front/app/components/ui/layout-contract.ts`（新增档位表与 helper）、`front/app/components/ui/AppDialog.vue`（默认 zIndex 取档位，数值不变）、`front/app/features/tags/components/CompositeLabelEditDialog.vue`（新增 `zIndex` prop + 遮罩内联层级）、新增/扩展三个前端单测文件。
- **文档**：`docs/reference/standard/frontend/layout.md`（增层级契约节）、`openspec/specs/unified-dialog/spec.md`（归档时同步 Requirement + API/Constraints 段）。
- **后端 / 数据库 / API**：零改动（纯前端层级与契约问题，不触任何接口、SQL、迁移）。
- **部署后用户可见变化**：治理工作面「组合」页签点「创建并挂载」，组合标签创建弹窗正常出现在工作面之上（此前完全看不见）；标签池页（总览）里的同一弹窗行为不变。
- **旧数据降级**：不涉及数据，无迁移、无需回填、无需重新生成；回滚前端构建即可完全还原旧行为。
