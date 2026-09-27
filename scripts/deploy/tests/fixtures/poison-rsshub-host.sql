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
-- ↓↓ 篡改注入：RSSHub 改写源 host 残留（占位示例域，非真实基础设施）
INSERT INTO feeds (id,url,icon,created_at) VALUES
('900','http://rss.example.internal:1200/36kr/information/web_news','https://favicon.example/icon?domain=rss.example.internal:1200','2026-09-20T08:00:00+08:00');
