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

-- ↓↓ 篡改注入：ai_providers 行携带非空 api_key（凭据泄漏）
INSERT INTO ai_providers (id,name,provider_type,base_url,api_key,model,enabled,timeout_seconds,max_tokens,temperature,enable_thinking,metadata,created_at,updated_at) VALUES
('1','zai','zai','https://api.z.ai','sk-SECRET-DO-NOT-SHIP-abc123','glm-5.3',true,60,4096,0.7,true,'{}','2026-09-01T10:00:00+08:00','2026-09-01T10:00:00+08:00');
