## 1. 用例与实施准备

- [x] 1.1 完成并审核 test-cases.md 故事、五组变体、白盒分支与旧Scenario继承表；验收：文件存在且两份spec的每个Scenario均有落点，不把待实现标成通过。
- [x] 1.2 保存本change文件归属和并发基线，读取apply指令；验收：执行 `bash scripts/concurrency-status.sh improve-discovery-recommendations` 并记录冲突文件，后续不覆盖其他change改动。

## 2. 候选数据与迁移

- [x] 2.1 新增候选实体、稳定身份与有效资料覆盖规则，原生URL限500字符；验收：候选纯逻辑测试通过，新增/重复/人工覆盖有断言。
- [x] 2.2 新增run、run items、interest entries、candidate preferences及向量模型，接入迁移并修复pending部分唯一约束；验收：隔离PG迁移重跑和NULL桶并发唯一性测试通过。
- [x] 2.3 回填旧路由与推荐，旧seed保存inactive历史，旧pending转legacy历史，保存回滚排除策略；验收：隔离PG升级/重跑/回滚故事通过且feeds/articles计数不变。

## 3. 目录服务与网络边界

- [x] 3.1 候选增改查、分页筛选与推荐启停API；验收：handler测试确认入库不订阅，失败输入不产生记录，enabled不改变长期排除。
- [x] 3.2 版本化JSON导入预览/确认及脱敏导出；验收：混合失败、revision冲突、重复重试、私有和query URL默认排除测试通过。
- [x] 3.3 实现有界安全RSS fetch及逐跳DNS/私网授权校验，接入实际端点可用性状态机；验收：本地fixture服务器测试超时、429、410、非RSS、重定向越界、未授权私网不发请求。
- [x] 3.4 修复同步失败误成功及url/example遗漏，人工覆盖隔离，按有效指纹增量重嵌；验收：旧revision结果不覆盖新资料、失败保留兼容向量、模型不一致阻断测试通过。

## 4. 发现与订阅

- [x] 4.1 复现并修复精排不筛选、Ask忽略错误和缺ID；实现run原子发布；验收：先有旧行为复现测试，再验证子集/零选择/未知ID/故障无发布及request_key重试。
- [x] 4.2 实现仅board匹配、独立种子、窗口与成熟度衰减配额；验收：test-cases.md边界值、单调性、有界份额测试通过。
- [x] 4.3 实现基础8+行为8双路召回、全局/查询上下文及同模型校验；验收：基础保底、去重来源、单路缺失、不混模型测试通过。
- [x] 4.4 实现过期、冷却、长期排除、恢复和刷新时间；验收：并发排除后旧run不能发布、截止两端、跨source隔离测试通过。
- [x] 4.5 提取共享建源服务，推荐accept安全验证、最终URL限制及事务去重；验收：原生/填参失败不建源，重复accept仅一个feed，参数字典及官方文档旧测试回归通过。
- [x] 4.6 注册检查/过期维护任务与分析暂停、分批回补、v2开关；验收：Registry单job互斥、accepted=false、pause和切换测试通过。

## 5. 前端按批准原型实现

- [x] 5.1 API类型与三页签、contained布局、双主题、目录搜索和编辑；验收：组件测试覆盖保存不订阅、启停不取消订阅和错误恢复。
- [x] 5.2 常驻查询、独立run结果、返回推荐、刷新、兴趣记录和历史排除；验收：组件测试覆盖查询空白/重复提交/失败保留输入，字段来源与状态契约。
- [x] 5.3 真实文件导入/下载及预览确认、敏感项提示、原生/RSSHub订阅弹窗；验收：组件测试覆盖取消无副作用、部分失败和填参兜底。

## 6. 测试

- [x] 6.1 运行 `bash scripts/change-scope.sh` 确认影响包，逐包执行测试（SQL仅隔离PG）；验收：保存精确命令和结果，无全库清理操作。
- [ ] 6.2 按test-cases.md执行opencli完整新增→入库→确认订阅、查询→返回、导入→重试和排除→恢复故事；验收：保存步骤断言及结果，不用静态原型代替真实页面测试。
- [ ] 6.3 对日本新闻、AI、开发工具三版块做只读候选效果对照；验收：记录基线/双路候选数量、重复率、基础路是否保留、精排零结果率及结论，无虚构数字。
- [x] 6.4 聚焦review迁移、事务、网络权限和旧入口绕过路径，修复High问题；验收：review记录与复测命令存在。

## 7. 文档

<!-- doc-impact: flow, api, database, architecture, configuration, deployment -->

suggest在代码尚未改动时仅命中其他change的configuration；本change按计划显式声明以上域，不将其他change文件纳入本次交付。

- [ ] 7.1 更新discovery/ai-summary/scheduler流程、API、数据库与架构索引；验收：`bash scripts/doc-impact.sh verify` 与 `bash scripts/check-standards.sh` 通过或明确独立环境/并发阻塞。
- [ ] 7.2 更新配置与部署说明（默认值、重嵌、旧历史、私网确认、回滚边界）；验收：文档列出部署后行为、人工操作和旧数据降级。

## 8. 验证

- [ ] 8.1 `openspec validate improve-discovery-recommendations --strict`；期望：valid。
- [ ] 8.2 在backend-go运行 `golangci-lint run ./...`、`go vet ./...`、`go build ./...` 及影响包 `go test`；期望：成功，测试包清单由change-scope确定并回填记录。
- [ ] 8.3 在front运行 `pnpm lint`，通过Windows cmd执行 `pnpm exec nuxi typecheck`、影响文件 `pnpm test:unit`、`pnpm build`；期望：成功，保留具体影响文件命令。
- [ ] 8.4 人工：真实页面在1440×900、1920×1080及390×844检查无横向溢出、dialog<=92vw、桌面contained<=1120px；保存截图及批准原型差异说明。
- [ ] 8.5 人工：根据下表逐条核对Scenario落点，并将拟定测试文件替换为真实通过的文件与测试名；任何未验证项保持未完成。

Scenario→测试文件映射由本次spec清单生成追加，路径是拟定落点，不代表测试已存在或通过。

| Scenario | 测试文件 |
| --- | --- |
| 手动查询完整流程 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 输入错误和重复提交 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 查询失败后恢复 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 精排选择子集 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 零选择不是故障 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 服务或协议异常 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 多主题查询不互相平均 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 有界衰减 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 行为成熟后让位 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 近期阅读不能挤掉版块方向 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 单路缺失与重复候选 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 模型不兼容 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 刷新与过期 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 冷却边界及长期排除 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 恢复不是订阅 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 原生与参数订阅 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 多来源和缺失检查信息 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 错挂种子的升级 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 响应包含 param_options | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 原生候选无参数表单 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 表单提供文档链接 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| doc_base 可配置 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 配置获取失败及原生来源 | backend-go/internal/admin/service/discovery_v2_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 手动入库再订阅 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 输入校验及同源重复 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 停用与订阅独立 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 人工说明经同步保留 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 上游删除或同步失败 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 导入后仍未订阅 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 混合结果与重试 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 不兼容版本 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 默认安全导出 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 导入私网地址 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 重定向改变访问范围 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 说明变化才重建 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 更新失败与模型切换 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 需参数但无实例 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 短暂故障和内容错误 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 修复后复查 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 空库与筛选无结果 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
| 长文本与双视口 | backend-go/internal/admin/service/candidate_catalog_test.go（拟定，待实现；UI故事另见test-cases.md） |
