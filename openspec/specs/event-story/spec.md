<!-- domain: topic-graph -->

# event-story Specification

## Purpose
定义**事件线**（`board_stories`）与**事件**（`daily_report_threads`）的身份关系：一条事件线聚合多天的多个事件。目标是把「今天这条话题有几件事、哪件事有几个来源在报」变成可查询的事实，并让跨期去重有稳定的判定依据。身份判定以确定性规则为主、模型只处理指纹缺失或冲突的窄区间；向量只用于召回候选，不参与判定。术语与图谱见 [`../data-map.md`](../data-map.md)。

## Requirements

### Requirement: 事件指纹结构化产出
标签提取路径 SHALL 为每个 `category='event'` 的标签同时产出结构化指纹：`subject`（主体原文：公司/机构/人物/组织）、`subject_key`（归一化实体键，格式 `类型:规范名`，类型限于 `person` / `company` / `org` / `product` / `other`）、`action`（动作：发布/宣布/涨价/回应/下架等动词短语）、`object`（对象：产品/政策/资产/事项）。`occurred_at` MAY 产出，缺失时 SHALL 由该标签关联文章的最早 `pub_date` 代替。指纹 SHALL 与标签在同一次 LLM 调用内产出，SHALL NOT 为生成指纹新增独立调用。

`subject_key` SHALL 由模型在同一次调用内完成别名归并，使同一实体的不同写法（如 `Trump` / 川普 / 特朗普 / Donald Trump）产出同一个键。`subject_key` SHALL NOT 依赖 `topic_tags.aliases` 做归并——实测 person/event 的 aliases 覆盖率为 0%、keyword 仅 7.3%，不足以支撑。

指纹 SHALL 与已有 `topic_tags.metadata.event_keywords` 做一致性校验：`subject` 与 `object` 均 SHALL 能在该标签的 `event_keywords` 中找到（允许包含关系与别名展开）。校验不通过的标签 SHALL 标记为「指纹不可信」，且 SHALL NOT 阻断标签本身的入库与既有聚类流程。

#### Scenario: 同一次调用产出指纹
- **WHEN** 某栏目片触发一次融合提取调用并返回 3 个 event 标签
- **THEN** 每个 event 标签 SHALL 携带 subject/subject_key/action/object；系统 SHALL NOT 为该片发起第二次指纹提取调用

#### Scenario: 指纹与关键词一致
- **WHEN** 标签「阳光电源股价因业绩反转预期及政策利好暴涨」的 `event_keywords` 含「阳光电源」「股价暴涨」，模型输出 subject=阳光电源、object=股价
- **THEN** 该标签指纹 SHALL 标记为可信

#### Scenario: 指纹校验失败不阻断标签
- **WHEN** 模型输出的 subject 不在 `event_keywords` 中且无法通过别名展开匹配
- **THEN** 该标签 SHALL 照常入库并参与既有聚类，指纹 SHALL 标记为不可信并记录 warning；后续归属判定 SHALL 走降级路径

#### Scenario: 无关键词可校验时降级
- **WHEN** 某 event 标签的 `metadata.event_keywords` 缺失（存量约 0.3%）
- **THEN** 该标签指纹 SHALL 标记为不可信，SHALL NOT 因缺少校验依据而丢弃标签
