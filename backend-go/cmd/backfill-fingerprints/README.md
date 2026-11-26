# 历史 event 指纹回填

从 `backend-go/` 运行；配置沿用 `configs/` 和现有 topic-tagging 路由，数据库 schema 必须已就绪。命令直连 DB，不迁移、不 seed、不启动服务。

```bash
go run ./cmd/backfill-fingerprints --dry-run --limit 100
go run ./cmd/backfill-fingerprints --board 7 --limit 100 --workers 4 --attempts 3
```

- `--board 0`（默认）：全部 event 标签，包括未归属版块的标签；指定 ID 时用 `topic_tag_board_labels` 筛选，不重复处理跨版块标签。
- `--limit 0`（默认）：不限；正数只限制待补指纹的标签数，不让跳过的标签吃掉额度；按标签 ID 升序选取。
- `--workers 4`（默认，范围 1–16）：标签级并发；每次 AI 调用超时 2 分钟。
- `--attempts 3`（默认，范围 1–5）：含首次尝试，失败后分别等 1、2 秒再试；airouter 自己的 provider failover 可能增加实际供应商请求数。
- `--dry-run`：严格不写库、**不调用 AI**。输出 planned / skipped / keyword_basis，模式固定为 `(mode=fill)`；可信率只能标为 unknown，额外提供“关键词覆盖率上限”，**不能把覆盖率当实测可信率**。

每 100 条输出一次进度；终态打印 ok / skipped / failed / trusted / elapsed。单条最终失败输出标签 ID 和错误；CLI 自身 JSON 解析失败、字段不完整时，附带原始响应前 200 个字符（引号转义换行），不输出完整 prompt（AI 调用按 airouter 规范留账）。airouter 内部结构化 `parse_error` 不返回原始响应，现有 `ai_call_logs.response_snippet` 在此分支也只记录错误及可选 `finish_reason`，无法通过 CLI 或审计表还原该响应；后续需在 airouter 的 `parse_error` 分支补原始响应截断日志。失败不阻断其他标签；有失败或收到 SIGINT/SIGTERM 时退出非 0，已提交的数据保留。

## 写入与兼容

AI 只读 label / description / event_keywords / aliases：description 按 Unicode 字符截到 200 字，event_keywords 最多 20 项，aliases 最多 10 项，不传整行 metadata。输出沿用提取器的五字段契约，但 CLI 仅设置 `JSONMode=true`，不再传 `JSONSchema`，不改动提取器共用的 `EventFingerprintSchema()`。prompt v4 保留直接输出 JSON、不输出推理过程或解释；将 v3 的「类型:规范名」收紧为「单一具体实体名」，按人/公司/组织/产品型号/其他专有实体区分，要求最短可识别形式，不列举主题词黑名单。正反例包括 `person:特朗普` / `person:美国总统特朗普`、`product:DeepSeek` / `product:DeepSeek Harness`、`org:美国白宫` / `org:美国白宫与俄罗斯联邦安全局`（单一实体与两个实体的区别）；完整 JSON 示例的 `subject_key` 改为 `person:特朗普`，无明确日期时 `occurred_at` 填 `""`；拿不准填空字符串，不猜测。参数保持 `maxTokens=1024`、`temperature=0.2`。此前受控实验表明原 schema 会使主体键循环生成并耗尽预算，仅 JSONMode 配合示例可返回正确格式；本次开发验证不再调用真实 LLM，不能保证所有标签的完成长度或准确率。解析与可信度判断直接调用 `service.ParseEventFingerprint`（内部复用 `ValidateEventFingerprint`、`textutil.NormalizeSubjectKey`）。

写入发生在**单标签事务**内：PostgreSQL `SELECT FOR UPDATE` 后重新检查指纹、用最新 keywords/aliases 判信任、保留其他 metadata。仅更新嵌套 `event_fingerprint` 和平铺 `fingerprint_trusted`，不修改标签状态或已有事件线。事务失败只回滚该条。

已有非空指纹（包括不完整、不可置信的旧指纹）不覆盖；NULL / 空对象 / 空白字符串视为空。成功后重跑不再调用 AI；其他进程已抢先写入时跳过。不同 CLI 同时运行仍可能在写入前重复调用 AI，但不会覆盖已有非空指纹，因此运维建议只跑一个实例。

主体是否具体由 prompt 的实体契约说明，不通过词表黑名单、长度阈值或头衔检查判断。结构缺字段 / JSON 解析失败会重试，最终计 failed；完整但关键词一致性校验失败仍保存并标记 `fingerprint_trusted=false`，不冒充可信。未补/不可信旧数据继续使用既有降级路径，不清理、不迁移。新指纹供后续归属判定使用，**不会自动重建历史 story 归属**。

## 部署后影响与操作

本次撤回 CLI 的黑名单、长度与头衔粒度校验及选择性重做标志；prompt v4 保留单一具体实体、最短可识别形式与正反例契约。在线归属判定同时收紧：规则合并要求对象归一化后严格相等；阶段合并要求发生日（缺失时取代表线程日报日期）相差不超过一天，日期证据不足则新建。已有事件线不自动拆分，旧指纹不自动重写，不要求用户手动执行操作。缺失指纹如需补齐，须另经审批后先 dry-run 再小批运行；真实写入前确认备份。本次开发验证没有连接真库、调用真实 AI、运行回填或重启服务。

此前记录的 9,439 个缺指纹标签（历史估算，不代表当前存量）：正常约 **9,439 次逻辑 AI 调用**；默认最多 28,317 次（含 CLI 重试，不含路由 failover）。4 并发、平均每次 2/5/10 秒时，理想耗时约 **79 分钟 / 3.3 小时 / 6.6 小时**，另加限流、重试、数据库开销。该估算并非实测；建议用首批 100 条 elapsed 校准。dry-run 为 0 次。

