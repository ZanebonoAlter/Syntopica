# 站内搜索（Search）

> 三路召回（pg_trgm 关键词 / identity 标签向量语义 / LLM 查询扩展）+ RRF 融合排序的聚合搜索端点。业务链路与约束见 [flow/reading.md](../flow/reading.md) §站内搜索（约束 #10-#12）。

## 搜索 Search

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/search` | 混合搜索聚合（关键词 + 语义 + 可选 LLM 扩展） |

### GET /api/search

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `q` | string | 是 | 查询串；空/纯空白（含全角）→ 400；>200 字符截断（记 warn）；LIKE 通配符（`%` `_` `\`）转义按字面匹配 |
| `archived` | string | 否 | `include`（默认，含归档行）/ `active-only`（仅活跃） |
| `expand` | string | 否 | `off`（默认）/ `on`——开启后 LLM 产 5~8 扩词补跑字面召回，结果区隔为扩展命中 |
| `page` | int | 否 | 页码（≥1，非法 → 400） |
| `per_page` | int | 否 | 页大小（≥1，非法 → 400） |

```json
{
  "results": [
    {
      "id": "123",
      "title": "特斯拉Model Y地下车库自燃",
      "excerpt": "……",
      "archived": true,
      "hit_reasons": [
        { "type": "keyword", "text": "特斯拉", "score": 0.143 },
        { "type": "semantic", "text": "电动车安全", "score": 0.83 }
      ]
    }
  ],
  "semantic": { "skipped": false, "tags": [ { "label": "电动车安全", "score": 0.87 } ] },
  "expand": { "words": ["电池热失控", "新能源车起火"] },
  "total": 67,
  "page": 1,
  "per_page": 20,
  "took_ms": 312
}
```

- **结果项**：沿用列表窄投影（无 `content`/`firecrawl_content`/`ai_content_summary` 等正文大字段，`excerpt` ≤200 字符，见 [articles.md](articles.md) 列表契约）+ `archived` 标记 + `hit_reasons`。
- **`hit_reasons[].type`**：`keyword`（L0-L3 字面管线命中，text 为命中词、score 为层级加权分或 trigram 相似度）/ `semantic`（text 为标签名、score 为 cosine）/ `expand`（LLM 扩展词命中）。
- **`semantic` 块**：embedding 不可用或 3s 超时 → `{ "skipped": true }`（HTTP 200 不报错，前端降级提示条）；无 identity 向量命中 → `skipped: false` + 空 `tags`。
- **`expand` 块**：仅 `expand=on` 且 LLM 成功时存在；LLM 失败不影响主结果，扩展区缺失。
- **性能**：关键词路径纯 SQL（pg_trgm GIN 索引）永远在线；语义路径并行执行不阻塞首屏（P50 < 1s 目标）。

### 配置键（ai_settings）

| 键 | 默认 | 说明 |
|----|------|------|
| `search_trigram_threshold` | `0.10` | L3 trigram 相似度阈值；非法值回退默认并记 warn |
| `search_semantic_min_cosine` | `0.55` | 语义路径标签 cosine 下限（与 top_n 双重约束） |
| `search_semantic_top_n` | `8` | 语义路径命中标签数上限 |
