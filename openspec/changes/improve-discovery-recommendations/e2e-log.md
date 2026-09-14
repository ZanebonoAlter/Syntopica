# 6.2 opencli 端到端验证日志（进行中）

环境：后端 Windows go run :5000（新代码，开发库迁移已执行：7 新表/3097 候选/5 legacy 种子/160 legacy pending/feeds=23 不变）；前端 Windows pnpm dev :3000；opencli v1.8.7 桥绿。

## 已验证 ✓

1. **三页签渲染**：/discovery 加载，h1「发现订阅源」，tablist=为你推荐/候选源库/兴趣记录。
2. **为你推荐页签**：常驻查询区「找订阅源」+「刷新推荐」+ 当前推荐/历史切换按钮齐。
3. **候选库列表**：30 条/页真实回填数据、手动新增/导入/导出工具栏、逐条编辑/订阅按钮。
4. **API 直连创建链路**（页面内 fetch POST）：200、候选落库、message="candidate created (not subscribed)"、subscribed=false、**feeds 计数不变**（入库≠订阅，spec C1 核心断言过）。
5. **UI 表单新增全链路**（契约修复后）：填表→v-model 绑定（读 setupState 证实）→submit→弹窗关闭→toast「尚未订阅」→列表入库→**feeds=23 不变**。
6. **重复地址**：同 URL 二次提交→后端 409→弹窗保留提示「候选库已有这个地址，不会重复新建」→不建第二条（计数仍 1）。

## 发现的真 bug（已派 contract-fix）

- ~~create/update payload 字段断裂~~ **已修**（url→feed_url、PATCH 补地址编辑、对账全绿）
- ~~create 不吃参与推荐开关~~ **已修**（补 recommendation_enabled 三态）

## E2E 第二波发现的真 bug（ask 预检/回填）

- **预检误杀**（ask-preflight-fix 修）：requireCapabilities 要求非 Ollama provider api_key 非空，但本环境 qwen/qwen3-embedding 走本地网关合法无 key（同 provider 成功嵌 949 条是铁证）→ ask/refresh 全落 configuration。15.8ms 内失败、零日志痕迹。
- **回填永败循环**（同上修）：有效文本 rune 上限 1800 超 embedding 供应商 512 token 上限（日志：input 794 tokens too large），部分候选永远嵌不上。收紧预算到总≤500 rune + 指纹版本 v2 强制重嵌。✅已修：预检 key 检查已删（补了无 key 网关正向用例）、failRun 落 cause 日志（立即立功：抓到下面的 context canceled）、文本预算 80/80/80/257+v2。
- **ask/refresh 同步执行**（async-run-fix 修）：POST /ask 同步跑全链，本地网关慢时 74s，前端超时掐 ctx → run 半途失败（新 failRun 日志抓到：`error_code=embedding err=...context canceled`）。设计 D2 是异步意图，前端也真在轮询；handler 改秒回 run_id+running，执行进后台 goroutine（5min 超时）。
- 小缺口待修清单：409 existing_id 未透到前端 dupHint（无跳转入口按钮）；?tab=candidates 刷新后不生效；前端 PATCH 不发 revision（乐观锁未启用）；dismiss 响应 snoozed_until 被丢（历史页另走 GET 可接受）。

## opencli 经验（补充 ui-verify skill 未记的坑）

1. **eval 读 innerText 匹配错误文案会撞标签**：「名称必填」是 label 星标（`<span class="ced__required">必填</span>`）不是校验错误；断言错误要匹配组件源码里的真实错误文案（如「名称不能为空」）。
2. **fill/type 的 verified/typed 只证明 DOM value 落上，不证明 Vue ref 收到**；判定绑定要用 `__vueParentComponent.setupState` 直读或看网络请求 payload。
3. `?tab=candidates` URL 参数未生效（默认仍落为你推荐）——疑似 5.1 的 tab query 支持只认特定值或刷新丢失，待查（低优先）。
4. el.click() 对 AppButton 偶发不触发 handler（skill 已记，本次再证实：同一路径 3 次里 1 次静默失败）。

## 待验证（契约修复落地后）

- UI 表单新增候选全链路（填表→保存→toast 尚未订阅→列表出现→feeds 不变）
- 编辑地址（rss 改 URL→revision+1→撞已有 409 已存在入口）
- 订阅流（原生确认地址→安全验证→已订阅防重复）——需可达的真实 feed，候选：复用库里已有 23 个订阅的地址做「重复订阅复用」断言
- 查询流（ask→run 轮询→独立结果→返回）——依赖 embedding/精排配置
- 暂时不看/长期排除/恢复 → 历史四状态
- 导入导出 UI 流

## 测试数据清理备忘

开发库现有 E2E 测试候选：id 55757（e2e-direct.xml）。收尾时统一清理 LIKE '%e2e-%' 候选与关联行。
