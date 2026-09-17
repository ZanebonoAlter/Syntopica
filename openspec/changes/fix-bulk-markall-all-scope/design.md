<!-- complexity: simple -->

## Context

存量 bug：`handleMarkAllRead`（FeedLayoutShell.vue）在「全部文章」视图（无 selectedFeed/selectedCategory）走 `markAllAsRead()` 无参分支 → bulk-update 无 scope → 后端 400（dc50062a，2026-06 硬化）。无主 spec 锚，本 change 新建 `article-bulk-update` capability 补契约。

## Goals / Non-Goals

**Goals:**
- 后端 bulk-update 支持 `all: true` 显式全站 scope，语义互斥校验（all 与其他 scope 冲突 → 400）
- 前端无 options 分支传 `all: true`，打通「全部文章」视图按钮
- 复现测试先行（无 scope → 400 现状断言红 → 修复后绿）

**Non-Goals:**
- 不改 ids/feed_id/category_id/uncategorized 既有语义
- 不为 favorite 提供全站入口（无产品诉求）
- 不做「撤销全站标已读」

## Decisions

1. **显式 `all` scope 而非"无 scope 放行"**：保留误操作保护（无 scope 仍 400），全站必须显式声明；备选"read=true 且无 scope 即全站"会把静默全站语义藏在缺参里，不可取。
2. **互斥校验**（all + 其他 scope → 400）：防止参数组合歧义，而非静默取优先级。

## Risks / Trade-offs

- [旧客户端隐式无 scope 请求仍 400] → 本就是现状行为，不变
- [全站标记行数大时 UPDATE 耗时] → 单用户数据量 ~2k 行，无索引问题

## Migration Plan

无迁移；前后端同批部署（老前端 + 新后端兼容：老前端无 scope 仍 400，与新后端一致；新前端 + 老后端：all 字段被忽略 → 仍 400——需同批上线，验收后一起部署）。

## Open Questions

无。
