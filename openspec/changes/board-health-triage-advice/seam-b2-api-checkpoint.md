# B2-API 额度阻塞检查点（历史现场，现已续接交付）

> **当前状态已更新**：MiniMax线程 f3db5215-595a-45c4-bde5-2b3dfb8d8fd3 已完成2.2及三P2验证，正式进度7/29。请以 seam-b2-api-handoff.md 为当前实现接缝，以下只保留故障与续做现场，不再据此重做已完成工作。

## 失败与状态

run=`5fc50243-679e-43ca-aee0-d75fb03f3757`，源B1 run=`92679f0f-c064-4065-aca2-085bc9af1285`，模型zai-coding-cn/glm-5.3:max。

服务返回429/code1308：已达到5小时使用上限，提示重置时间 `2026-10-29 03:07:28`（原文未标时区）。原生进程状态failed/terminal observed，无pending supervisor请求。父线程未循环重试或私自换模型。

正式任务仍6/29完成；2.2未勾选，seam-b2-api-handoff.md不存在。A/A2及B1原已验证结果不因本次provider失败失效，不重新实施它们；本批新增代码尚无新的成功验证记录，不能将三个review问题标已修通过。

## 已落盘（只读核对，不覆盖回滚）

- repository/auxiliary_description_repair.go 已有锁前幂等回放、key/expected_version容量校验的B-P2-1/3代码标记；对应repo测试文件有新增/编辑，未验证。
- platform/database/auxiliary_description_repair_migration.go 已抽 `ensureAuxiliaryDescriptionRepairMutexIndex`；migrator.go 已在AutoMigrate成功后调用，属于B-P2-2自愈代码，尚无新增测试通过证据。
- 新建 service/auxrepair/auxrepair.go（179行）：薄service/请求与view类型、NewService显式DB、context/创建/查询方法已落盘。要继续复核完整性，不能假定已编译或接口已通。
- handler/auxiliary_description_repair_handler.go 与对应HTTP测试尚未创建；生产route/worker/原子保存/匹配/前端未接。
- service/service.go虽在Git状态为modified，但当前未发现repair facade导出，不把其既有脏改认作本批交付，更不可覆盖。

## 当前指纹（只是检查点，续做时重读并保留后来的合法改动）

| 文件（backend-go/internal/ 下） | SHA256 |
| --- | --- |
| tagmanagement/repository/auxiliary_description_repair.go | bdd1450383848eb9e5b2872d2a43986b624e58187713341bd08dbf8257c90a57 |
| tagmanagement/repository/auxiliary_description_repair_test.go | b5fded7d35d2796254a56484b1c539c5e852d17f2cf074105352556fe0d63263 |
| platform/database/auxiliary_description_repair_migration.go | aa3eebfce514d2e5cd52a0bba373d35e6b1f8394e098d27b7409f5e3755689e2 |
| platform/database/migrator.go | 42c3e64db2511d8b0ca64df480b6f24c8ef6eee00748c923e56cd7feaeb3d6ef |
| tagmanagement/service/auxrepair/auxrepair.go | e12d218941645328994ebcfc491065bb34da8337f869992c6dd6126407860940 |
| tagmanagement/service/service.go（含外部既有改动） | 5f9ffa4951ff8832ed683a2884bc31575015ccb6cfce87dc28376cfff92f8589 |

原始本批日志：`/tmp/pi-subagents-uid-1000/async-subagent-runs/5fc50243-679e-43ca-aee0-d75fb03f3757/`；保留会话中当前批次命令，不把源B1旧验证误算为本批新验证。

## 续做边界

先保留并复核已有改动，完成本批尚缺的repo/索引自愈回归、service/facade/三HTTP端点及测试。仍不做worker/原子保存/前端或生产接线，不更改用户已批准UI。三P2必须真正修复，不以文档化例外代替；按implementation-contract.md和review.md执行。新/变测试按最小范围跑，旧已通过PG/外部门禁不重复执行。Git写操作仍一律禁止，不stash/reset/restore/clean/add/commit；不写真库、不全局gofmt、不部署。

模型选择已获用户明确决定：「换minimax-3.1 flash max做」。静态models.json可见minimax-cn，但旧会话原生registry启动前未识别。随后commandcode路由尝试于首个模型请求即401（run ce571635-1043-406c-be3d-03ed8a2a6441，工具执行数0），没有改写源码。用户再次明确目标就是`minimax-cn/MiniMax-M3.1-Flash:max`，不再使用commandcode。根据本机Pi docs/models.md，打开/model会重读models.json；用户已reload，原生delegate run `acbe3a20-78c7-47d4-8e0e-653fa5b3820c` 以该精确ID/max成功收到 `MINIMAX_CN_READY`（无仓库工具操作），注册与实际鉴权已验证。现只以用户这个确切provider/model按原生协议接续；旧retained run固定GLM模型，不在resume中偷换。无需等待GLM重置，不切CLI、不回滚/重写已完成代码、不重跑已通过旧测试。此决定不改变UI原型批准、实现范围或验收门禁。

## 暂停后的最小回归修复（父线程，非整批实现接管）

用户增量门禁报告新auxrepair文件models导入未起tagmodels别名，导致typecheck/vet/build/short回归。父线程只补该一行import alias；门禁随后确认vet/build/tagmanagement short转绿。余下本批迁移文件gofmt回归，父线程仅gofmt该新迁移文件，用户门禁随后确认golangci-lint转绿。未重跑原有PG或整域测试，未改任何业务逻辑、未私自切模型继续整批。

修复后这两个文件的新SHA256（替代上表相应行，其他检查点不变）：
- service/auxrepair/auxrepair.go：`503e2b5bbc0b1fcea1ea925e30cb321da89a28cc82c0f40b379e1d8483253718`
- platform/database/auxiliary_description_repair_migration.go：`3d05ade2a7deb1cf2414969ff4de7a55b8256c06ee9bde4a5d38ab97c42cc077`

当前可构建/门禁绿不等于2.2功能已交付：新增P2修复专项PG、HTTP/service测试及handler仍待续做，2.2保持未勾选，正式进度6/29。
