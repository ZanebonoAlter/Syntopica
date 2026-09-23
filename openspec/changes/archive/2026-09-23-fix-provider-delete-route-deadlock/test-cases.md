# test-cases: fix-provider-delete-route-deadlock

测试单元（用户故事）：用户删除一个挂在能力线路上的备用 AI provider，系统自动解绑并删除成功；被摘空的线路保留且调用该能力时报可预期的错。复杂度：simple。

## 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | DELETE /providers/:id，provider 挂在 2 条线路（不同 capability） | 删除挂在线路上的 provider | 200；message 含 detached 数量；`ai_route_providers` 该 provider 关联=0；provider 行删除 | handler（HTTP） | `TestDeleteProviderDetachesLinkedProvider`（新增，先红后绿） |
| 2 | DELETE 无线路引用的 provider | 删除未被引用的 provider | 200 直接删除 | handler | `TestDeleteProviderRemovesUnusedProvider`（继承，不动） |
| 3 | 查询被摘空线路的可用 provider | 被摘空的线路允许存在 | 返回 `ErrNoProviders` | 函数（store） | `TestListRouteProvidersEmptyRoute`（新增，airouter store） |
| 4 | 页面删除挂线路 provider，确认弹窗→删除→列表刷新 | ui-design 验收映射 | 不调用 `removeProviderFromRoute`；调用 `loadData`；确认文案含「解绑」提示 | 组件（Vitest） | `useAIRouterSettings.test.ts` 新增用例 |
| 5 | 挂线路 provider 的删除按钮可点、行内有解绑告知文案 | 删除入口不被引用状态阻断 / 挂线路 provider 的删除入口与告知 | 按钮无 disabled；提示文案含「解绑」 | 组件（Vitest） | `AIRouterBackupProviders` 挂载断言（若该组件无既有测试文件，随 2b.1 组件测试一起补） |
| 6 | useConfirm 确认/取消/Escape | 删除确认弹窗 | Promise resolve true/false；danger variant 渲染 danger 按钮 | 组件（Vitest） | `useConfirm.test.ts` + `AppConfirmDialog.test.ts`（2b.1） |

## 继承与调整（⓪ 改契约）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 删除被线路引用的 provider 被拒 409（隐式契约，旧 spec 未成文） | 废弃 | `TestDeleteProviderBlocksLinkedProvider` | 改写为 `TestDeleteProviderDetachesLinkedProvider`（断言级联解绑），名称与断言全换 |
| 删除未引用 provider 成功 | 继承 | `TestDeleteProviderRemovesUnusedProvider` | 保留不动 |
| UpdateRoute 空 provider_ids 400 | 继承（Non-Goal，语义不变） | 无既有测试 | 不动、不补（不在本 change 范围） |

## 变体走查

**输入**：provider_id 非数字→400（既有行为不动，无新断言）；空串/空白/特殊字符→同上划除（URL 参数解析既有逻辑，本 change 不触碰）。

**前置**：空集（无关联 provider）→主链路步 2；单元素（挂 1 条线路）→主链路步 1 变体覆盖（2 条线路已覆盖多条，1 条由 message 数量断言=N 覆盖）；重复（同一 provider 挂同一线路两次）→`idx_ai_route_provider_link` 唯一索引（route_id+provider_id）物理防重，划除；越界引用（provider_id 不存在）→步 1 删除 provider 前 `First` 查不到→404（既有，不动）。

**时间窗口**：不涉及，划除。

**幂等**：重复删除→第二次 404 provider not found（既有 First 查询路径，无新代码）；部分失败重试→事务内两步同成败，无中间态；并发（仅当声称线程安全）→未声称，划除。

**可用性（UI）**：误输入反馈→不适用（删除是确认动作）；空态→删除后列表刷新由 `loadData` 既有逻辑覆盖；错误态→后端非 success 时 pushMessage error（既有分支保留）；加载态→`saving` 禁用按钮（既有）；超长文本/重复提交→不适用，划除。

## 效果核对

不涉及效果依赖断言外因素（数据覆盖率/LLM 行为），豁免。

## 白盒附加

simple 档，不适用。附实现分支说明：`DeleteProvider` 分支仅剩「查不到→404 / 事务删除→200 / 事务失败→500」三支，步 1、2 覆盖前两支，事务失败分支由既有 500 路径风格保证（不强行注入错误注入测试，划除留痕）。
