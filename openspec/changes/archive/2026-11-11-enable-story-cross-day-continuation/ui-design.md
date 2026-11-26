<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# 相关事件 UI 复用合同

## 入口与变更

1. 既有治理入口的「匹配规则」：`MatchingConfigDialog.vue` 追加独立字段「相关事件相似度」，默认 0.86（2026-11-10 S3 实测后由 0.82 上调），范围 0..1、步进 0.01。说明「按事件语义推荐相关内容，不合并事件」，不混入入板规则的命中即停顺序。
2. 归档文章标签 chip → `ArchiveMatchPopover.vue` 的 tag 模式：只有 category=event 时显示「相关事件」展开按钮，展开后按需查询同版块相关事件；article 模式与非 event 标签不增加请求。
3. 列表展示事件标签原名称、可展开的描述、相似度（非置信度或同一事件概率）。列表最多 10 条，不加阶段、趋势、发酵徽标。
4. 条目展开可查看完整描述；「查看来源」复用 `BoardConsoleShell.vue` 的现有文章预览链路。无来源文章则禁用该按钮并说明「暂无可读来源」，不拼造文章 ID。不递归自动展开相关事件。

## 受影响状态

| 状态 | 可见反馈与恢复 |
| --- | --- |
| 配置加载 | 沿用加载态，禁止未加载就提交默认值覆盖旧配置 |
| 配置非法 | 字段旁报错；NaN、Infinity、空值、范围外拒绝；保留输入 |
| 配置保存失败 | 保留输入和已展开内容；提示重试，不显示已保存 |
| 相关列表未展开 | 不查询、不生成 embedding，不阻塞原标签卡 |
| 查询中 | 局部 loading；切换标签、版块或关闭后丢弃迟到响应 |
| ready 且空集 | 「当前阈值与近30天范围内暂无相关事件」 |
| partial | 列出现有结果，同时显示已就绪/候选数及「部分事件尚未索引」 |
| not_ready | 「相关事件索引尚未就绪」，不能冒充空集；可重试查询 |
| 查询失败 | 局部错误与重试，原辅助标签、入板说明及阅读照常可用 |

## 复用组件与布局模式

- 不新增页面，保留现有 console workspace 及归档布局；不改日报叙事结构。
- 配置仍复用 AppDialog/AppInput/AppButton；本次触及配置弹窗时将旧自由 width 迁到标准 `size="lg"`（760px、92vw 上限），不另造弹层。
- 标签卡沿用既有 fixed/视口 clamp、窄屏底部形态和 layout-contract 的层级；相关列表在现有滚动区域内展开，不叠新侧栏。
- 长名称允许换行或 ellipsis+完整 title，描述展开后可读，来源预览关闭后回到原卡，不手写 z-index。
- 本次不改主流程/信息分区，故 minor；无需原型，不伪造 major 批准。

## 验收映射

| 故事 | 机械断言 | 证据落点（实现后已产生） |
| --- | --- | --- |
| 调阈值 | 打开→填0.86→保存→重开仍0.86；非法输入不发PUT；失败保输入；新查询返回生效阈值 | 组件测试 `front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts`（10 例）+ 后端 `backend-go/internal/tagmanagement/handler/related_events_test.go`（配置 6 变体）+ 浏览器 `ui-verification/report.md` A-1/A-2 |
| 看相关事件 | event卡展开发一次查询；只显示接口返回排序；展开描述可读；来源按钮打开对应文章 | 组件测试 `front/app/features/tags/components/console/ArchiveMatchPopover.relatedEvents.test.ts`（12 例）+ 浏览器 `ui-verification/report.md` A-3/A-4 |
| 局部降级 | loading/empty/partial/not_ready/error各态可分辨；非event不请求；切换后的旧响应不覆盖 | 组件测试 `front/app/features/tags/components/console/ArchiveMatchPopover.relatedEvents.test.ts` + 浏览器 `ui-verification/report.md` A-5 |
| 布局 | 1440×900、1920×1080及390×844；真实长标题下无横溢；elementFromPoint命中当前浮层；阅读弹窗仍可关闭返回 | 浏览器 `ui-verification/report.md` A-6（三视口机械断言 + 截图） |

浏览器验收已执行（2026-11-11），步骤/断言/截图见 `ui-verification/report.md`；组件测试 22 例见上述两个文件。保留历史提示：本合同签署时证据尚未产生，不能把合同本身当已完成测试。
