-- heal-dangling-article-refs 部署后核对（tasks 4.3 / 8.3）
-- 迁移 20260917_0002 随后端启动自动执行（幂等、不可逆）；部署后按下述查询逐条核对。
-- 用法：docker exec syntopica-postgres psql -U postgres -d syntopica -c "<query>"

-- ① 悬空引用必须为 0（修复前：5595 个引用，含 2026-09-16 日报的 9 条线索）
SELECT COUNT(*) AS dangling_refs
FROM daily_report_threads t
CROSS JOIN LATERAL jsonb_array_elements_text(
  CASE WHEN jsonb_typeof(t.related_article_ids) = 'array' THEN t.related_article_ids ELSE '[]'::jsonb END
) AS e(elem)
WHERE NOT EXISTS (SELECT 1 FROM articles a WHERE a.id::text = e.elem);
-- 期望：0

-- ② 非数组值（JSON null 标量 / SQL NULL）必须为 0（修复前：3 行）
SELECT COUNT(*) AS non_array_rows
FROM daily_report_threads
WHERE jsonb_typeof(related_article_ids) IS DISTINCT FROM 'array';
-- 期望：0

-- ③ 2026-09-16 那批线索的引用现状（修复前 9 条线索各有一个悬空 id）
SELECT t.id AS thread_id, t.related_article_ids
FROM daily_report_threads t
WHERE t.id IN (10173, 10181, 10186, 10188, 10190, 10212, 10227, 10264, 10274)
ORDER BY t.id;
-- 期望：数组里不再出现已删文章 id；10264 应保留 [129446]（keeper 仍在，来源可追溯）
--       仅剩死链的数组被剪成 []（前端不再渲染「文章 #<id>」降级条目）

-- ④ 归并路径的引用改指（新增行为，可抽查近 7 天合并过的组）
SELECT t.id AS thread_id, t.related_article_ids
FROM daily_report_threads t
WHERE EXISTS (
  SELECT 1 FROM jsonb_array_elements_text(
    CASE WHEN jsonb_typeof(t.related_article_ids) = 'array' THEN t.related_article_ids ELSE '[]'::jsonb END
  ) AS e(elem)
  WHERE NOT EXISTS (SELECT 1 FROM articles a WHERE a.id::text = e.elem)
);
-- 期望：0 行（与 ① 同口径，用于确认巡检口径与修复口径一致）

-- ⑤ 快照计数不回填（已知取舍，非问题）：article_count 是生成时快照，
--    剪除死链后「N 篇」可能大于展开列表长度，属预期，不在本 change 修复范围。
SELECT COUNT(*) AS sections_with_article_count FROM daily_report_sections WHERE article_count > 0;
-- 仅记录现状，无期望值
