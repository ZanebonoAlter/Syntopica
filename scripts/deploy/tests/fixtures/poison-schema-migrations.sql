-- Syntopica demo seed (sanitized).
-- Generated: 2026-09-24 10:08:13
-- Window: last 30 days. Schema is built by the app at startup (AutoMigrate + migrations);
-- this file only contains INSERT data and sequence resets.
INSERT INTO categories (id,name,slug,icon,color,description,created_at) VALUES
('3','ai新闻','48eba0b6','mdi:folder','#6366f1','','2026-06-14T16:09:11.310985+08:00'),
('1','技术','a95dd3e1','mdi:folder','#6366f1','','2026-06-14T16:09:11.191322+08:00'),
('2','新闻','1d9c15c5','mdi:folder','#6366f1','','2026-06-14T16:09:11.249249+08:00'),
('4','游戏相关','99cc558a','mdi:folder','#6366f1','','2026-06-14T16:09:11.325301+08:00'),
('5','论坛','a5f625b2','mdi:folder','#6366f1','','2026-06-14T16:09:11.334187+08:00');

-- ↓↓ 篡改注入：真库导出不应出现的 schema_migrations（会压制新库迁移）
INSERT INTO schema_migrations (version, driver) VALUES
('0007_create_daily_report', 'postgres');
