# test-cases — fix-bulk-markall-all-scope

测试单元：故事「全部文章视图点『全部标为已读』」（锚 Requirement: 批量操作 scope 契约）。complexity: simple（无白盒附加节义务；变体走查为义务）。

## 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 用户在「全部文章」视图（无 feed/category 选中）点 header「全部标为已读」 | 显式 all 全站标已读 | PUT bulk-update body `{read:true, all:true}` | 组件/store | app/stores/api.test.ts `markAllAsRead via articlesStore…`（断言 body 含 all:true） |
| 2 | 后端收到 `{read:true, all:true}` | 显式 all 全站标已读 | 200 + `{"message": N}`，全站 read 置位 | handler | backend-go/internal/reader/handler/bulk_update_test.go TestBulkUpdateAllScope |
| 3 | unread 归零（DB 断言 count=0） | 显式 all 全站标已读 | countUnread==0 | handler | 同上（测试内 DB 断言） |
| 4 | 视觉确认 toast 成功 + 未读清零 | 显式 all 全站标已读 | V.3 人工 | 人工 | dev 环境实测（模型无关） |

## 变体走查（五组清单）

| 变体组 | 走查 | 答案 | 落点 |
| --- | --- | --- | --- |
| 输入 | 空串/纯空白/分隔符/单 token/大小写/特殊字符 | 不适用（数值/布尔字段，无文本输入）——划除留痕 | — |
| 前置 | 无 scope 无 all | 400 "Must specify a scope"（契约保留，不放松） | TestBulkUpdateNoScopeStillRejected |
| 前置 | 仅 scope 无更新字段 | 400 "At least one field" | TestBulkUpdateScopeWithoutField |
| 前置 | all + feed_id 冲突 | 400 "all cannot be combined with other scopes" | TestBulkUpdateAllConflictWithFeedScope |
| 前置 | 空库 all（0 行） | 200 + message=0（RowsAffected 0，非错误） | TestBulkUpdateAllScope（seed 3 行覆盖非空；空库由 UPDATE 语义天然覆盖，行数=0） |
| 时间窗口 | 边界/空窗口 | 不适用（无时间语义）——划除留痕 | — |
| 幂等 | 重复执行 all:true | 二次执行 rows=0 仍 200（is_read 已 true 不变） | 同主链路实现，GORM Updates 幂等；人工 V.3 复点验证 |
| 幂等 | 并发 | 无并发入口（单用户单请求）——划除留痕 | — |
| 可用性 | 误输入反馈 | 400 文案可读（既有） | TestBulkUpdateScopeWithoutField |
| 可用性 | 空态/加载态/超长/重复提交 | 无 UI 结构变更（none）——划除留痕 | — |

## 效果核对

无（效果不依赖断言外因素——纯 UPDATE 语义，无需真库量化）。

## 白盒附加

simple 档：无白盒附加义务。分支表已由四个 Scenario 覆盖（all 存在性 × scope 冲突 × 字段存在性 = 分支表全列）。

## 继承与调整

无旧测试改契约（bulk-update 无既有 spec 锚与旧测试资产，`bash scripts/test-assets.sh article-bulk-update` 反查零命中——新增 capability）。
