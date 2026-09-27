# 页边注三表（`tables/margin-notes.md`）

> 日报页边注（批注锚点 / 问答轮 / 术语库）的表结构事实。来源 change：`openspec/changes/archive` → daily-report-margin-notes（2026-09-24 联网扩充含 `cited_web_sources` 列）。模型定义：`backend-go/internal/topicgraph/repository/margin_notes_repository.go`；迁移：`postgres_migrations.go` `20260922_0003` + `20260924_0001`。

## 表清单

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `report_annotations` | 批注锚点（划词 + 归属 + 偏移线索） | `topicgraph.ReportAnnotation` |
| `annotation_qas` | 问答轮（含引用/网络来源/术语 jsonb） | `topicgraph.AnnotationQA` |
| `term_notes` | 术语库（归一化唯一键，P2 预留 embedding） | `topicgraph.TermNote` |

## report_annotations

| 字段 | 类型 | 约束/默认/索引 | 用途 |
| ------ | ------ | ------ | ------ |
| `id` | BIGSERIAL | PRIMARY KEY | |
| `report_id` | BIGINT | NOT NULL + `idx_report_annotations_report` | 所属日报（GORM 逻辑关联） |
| `section_id` | BIGINT | NOT NULL | 所属分区（头条 lead 批注为 0，`ValidateAnnotationTarget` 放行 `thread_id=NULL && section_id=0`） |
| `thread_id` | BIGINT | NULL | 所属线索（lead 批注为 NULL） |
| `quoted_text` | TEXT | NOT NULL | 划词原文（重放定位主键；偏移失配时模糊匹配降级，再失配显示「原文已变更」） |
| `anchor_offset_start` / `anchor_offset_end` | INT | NOT NULL DEFAULT 0 | 字符偏移线索（非精确定位依据） |
| `created_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() + `idx_report_annotations_created` | |

## annotation_qas

| 字段 | 类型 | 约束/默认/索引 | 用途 |
| ------ | ------ | ------ | ------ |
| `id` | BIGSERIAL | PRIMARY KEY | |
| `annotation_id` | BIGINT | NOT NULL + `idx_annotation_qas_annotation` + DB 级 FK `fk_annotation_qas_annotation`（`ON DELETE CASCADE`，20260922_0003 创建，全库 6 条真实 FK 之外的合法新增——删除批注连带问答的 DB 级兜底） | 所属批注 |
| `question` / `answer` | TEXT | NOT NULL | 问答原文 |
| `cited_article_ids` | JSONB | NOT NULL DEFAULT `'[]'::jsonb` | 引用文章 id 数组：**空写 `[]` 不写 null、保序去重**（数组契约）；服务端按 thread 关联文章白名单硬校验（集外剔除，WS-3/EF-1） |
| `cited_web_sources` | JSONB | NOT NULL DEFAULT `'[]'::jsonb`（`20260924_0001` 追加） | 网络来源 `[{"title","url"}]`：url 必须在本次 SearXNG 搜索结果集内（防编造链接），保序去重；空写 `[]` |
| `extracted_terms` | JSONB | NOT NULL DEFAULT `'[]'::jsonb` | 本轮涉及术语（归一化字符串数组；service 回答时另带 `new_terms` 标识本轮新建） |
| `operation` / `provider` / `model` | VARCHAR(80)/(100)/(100) | NULL | 审计冗余（ai_call_logs 7 天清理后仍可按 provider/operation 对账）；`model` P1 写空值（ChatResult 不携带） |
| `created_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

## term_notes

| 字段 | 类型 | 约束/默认/索引 | 用途 |
| ------ | ------ | ------ | ------ |
| `id` | BIGSERIAL | PRIMARY KEY | |
| `term_norm` | TEXT | NOT NULL + UNIQUE `uq_term_notes_term_norm` | 归一化唯一键（trim + 全角→半角 + 大小写折叠） |
| `term_display` | TEXT | NOT NULL | 首次出现的原始形态（trim 后） |
| `hit_count` | BIGINT | NOT NULL（默认 1） | 遇到次数：同词再次问答 +1 不新建（WB-5） |
| `first_seen_board_id` | BIGINT | NULL | 首次遇到的版块 |
| `first_seen_date` | DATE | NULL | 首次遇到日期（问答当下，非报告 period） |
| `last_seen_at` | TIMESTAMPTZ | NULL | 最近一次命中 |
| `embedding` | vector(2560) | NULL DEFAULT NULL | **P2 预留不写入**（维度=2026-09-22 生产库实测核定，与 semantic_labels 等列同维） |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL | |

## 契约速查

1. **jsonb 数组契约**：`cited_article_ids` / `cited_web_sources` / `extracted_terms` 三列统一「空写 `[]` 不写 null」；null 标量/非数组在 `InsertAnnotationQA` 落库前拒绝（`canonicalize*JSONArray`，WB-7）。
2. **写入唯一路径**：`repository.InsertAnnotationQA`（白名单校验与归一化在 service `margin_notes_qa.go`）。
3. **幂等迁移**：三表 `CREATE TABLE IF NOT EXISTS` + FK 按约束名探测；`cited_web_sources` 用 `ADD COLUMN IF NOT EXISTS`（WS-9，testcontainer 复跑验证）。
