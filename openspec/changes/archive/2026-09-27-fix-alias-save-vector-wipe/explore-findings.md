
## addAlias 清向量 bug：代码锚点与数据基线

## 巡检证据链与修复锚点（2026-09-27 实测数据）

### bug 链路（代码定位）
- `backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go` L~533 `loadActiveAuxiliaryLabels`：`Select("id, label, slug, label_type, aliases, ref_count, description, status, protected, source, display_order, created_at, updated_at")` 排除 embedding/merge_embedding（17.5k 标签缓存内存预算）。
- `ResolveAuxiliaryLabel` L2 命中 → `addAlias`（L661）：`label.Aliases = append(...)` 后 `s.db.Save(label)` → GORM Save 全量保存，nil 指针 → 向量列写 NULL（status 仍 active，无报错）。
- `MergeAuxiliaryLabelAlias`（L277）：source/target 均 `tx.First` 全列加载 → `tx.Save(&target)` 当前恰好安全（结构体带向量），防御性改列级 Updates。
- 路由组：`board_crud_handler.go` L150 `auxiliary := rg.Group("/auxiliary-labels")`（实际路径 /api/auxiliary-labels，API 文档 docs/reference/api/semantic-boards.md 同名表）；回填端点挂这里，`POST /gc` 是同步先例。
- 回填生成规则（L3 同款，L232-249）：merge←`embedder(label, merge)`；storage←`embedder(label+": "+description 或 label, storage)`；embedder=`DefaultAuxiliaryLabelEmbedder`（可注入函数类型 `AuxiliaryLabelEmbedder`，fake 友好）。
- 测试基建：`testutil.SetupTestDB(t)` = testcontainers PG（internal/platform/testutil/testutil.go L172），auxlabel 包既有测试已用它。

### 数据事实（修复前后对比基线）
- 全库 active aux 死向量：1505/17525；11/12 版块构成含死向量共 124 个（刚果57%/中东43%/生成式AI34%/中国25%/美国20%/日本17%…），死的全是各版块 top-ref 标签。
- 死向量标签 100% 带 aliases、活向量 100% 无（addAlias 痕迹闭环）。
- 验证锚点行：id=113731 高市早苗（日本新闻构成，ref 47）、id=3238 日本央行（ref 23）。
- 受害时间线：updated_at 聚集午夜打标批处理；速率 9/16 前 4-8/天 → 9/21 后 60-300/天。
- 引入于 optimize-pg-storage 时代（2026-08-31~09-04，commit 5bf47b7e 前后）。

### 连带影响（部署后验证用）
- 日本新闻版块 maxSim 案例：tag「日本首相高市早苗在联大要求删除敌国条款」对日本构成 0.733（死向量截肢）→ weighted 0.44 拒收；对中国构成共享郭嘉昆 maxSim=1.0 → weighted 0.68 收留。向量恢复后日本构成 maxSim 应回 1.0（高市早苗 aux cos=1.0）。
- 语义版块红线 2（flow/semantic-board.md）即本 bug 的文档约束：L2 命中只 addAlias、不改 Label、不重算 MergeEmbedding。
- 不修范围：weighted 单共享标签后门（0.6×1.0+0.4/N≥0.68 恒过线）、direction 阈值 0.6 挡不住地缘相邻域（0.57-0.64 跨线）——后续独立 change。

<!-- pinned 2026-09-27T14:35:18Z -->

## GORM 列级更新 serializer 字段只有 struct 路径可用

GORM v2 写 jsonb serializer 字段（如 SemanticLabel.Aliases）只有 **struct 路径**（Save / struct Updates）经 field.ValueOf 应用 serializer；**map 路径**（Update("aliases", slice) / Updates(map)）的 raw value 直通 driver（callbacks/update.go map 分支 → clause.Assignment → AddVar，无 serializer 处理），[]string 交给 pgx 会因类型不匹配失败。要做「列级只更新某 serializer 字段」，正确姿势是 `Model(&models.SemanticLabel{}).Where("id=?", id).Select("aliases").Updates(models.SemanticLabel{Aliases: v})`——Select 限定 SET 集合只含该列，updated_at 由 AutoUpdateTime 照常自动维护。另一坑：pgvector 列存 float4，回读文本是最短表示（写入 "1.000000" 回读 "1"），断言「向量不变」不能逐字节比字符串，要解析后按元素 float32 比较（见测试 helper requireSameVector）。

<!-- pinned 2026-09-27T15:02:53Z -->
