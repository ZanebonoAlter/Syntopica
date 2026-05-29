## 1. 项目基础设施

- [ ] 1.1 创建 `tests/acceptance/` 目录和 `pyproject.toml`，配置 uv 项目（pytest>=8.0, playwright>=1.40, requests>=2.31）
- [ ] 1.2 创建 `tests/acceptance/conftest.py`，实现 session-scoped 环境就绪检查 fixture（backend localhost:5000 + frontend localhost:3000）
- [ ] 1.3 创建 `tests/acceptance/helpers/api.py`，实现 API client helper（GET/POST/PUT/DELETE + JSON 解析 + 错误处理）
- [ ] 1.4 创建 `tests/acceptance/helpers/browser.py`，实现 Playwright fixtures（browser/context/page）和导航 helper（navigate_to_tags）
- [ ] 1.5 创建 `tests/acceptance/helpers/selectors.py`，定义 /tags 页面的所有 CSS/文本选择器常量
- [ ] 1.6 创建 `tests/acceptance/helpers/__init__.py` 和 `tests/acceptance/__init__.py`
- [ ] 1.7 运行 `cd tests/acceptance && uv sync && uv run playwright install chromium` 验证环境搭建

## 2. unify-tag-hierarchy — API 验收测试

- [ ] 2.1 创建 `tests/acceptance/changes/unify-tag-hierarchy/` 目录和 `conftest.py`（API client fixture + 测试数据准备）
- [ ] 2.2 实现 `test_story_00_api_sector_crud.py`：POST 创建 manual sector → GET 列表验证 → DELETE 删除 → GET 验证已删除
- [ ] 2.3 实现 `test_story_00_api_hierarchy_config.py`：GET 获取配置 → PUT 修改 → GET 验证变更 → PUT 恢复原配置
- [ ] 2.4 实现 `test_story_00_api_rebuild.py`：POST 触发重建 → GET 轮询状态（5s 间隔，10min 超时）→ 验证 completed 或 skip
- [ ] 2.5 实现 `test_story_00_api_pending_changes.py`：GET 获取列表 → POST 批量审批 → GET 验证清理（无数据时 skip）

## 3. unify-tag-hierarchy — UI 验收测试

- [ ] 3.1 实现 `test_story_01_tags_page_load.py`：导航 /tags → 验证页面标题 "标签管理" → 验证分类切换器 → 验证三区域布局 → 验证默认 event 选中
- [ ] 3.2 实现 `test_story_02_sector_list.py`：验证 sector 列表展示 → 点击切换 category → 验证 "全部" 选项 → 验证 "添加板块" 和 "LLM 重新生成" 按钮
- [ ] 3.3 实现 `test_story_03_sector_create.py`：点击 "添加板块" → 验证弹窗 → 输入 label → 点击确认 → 验证新 sector 出现（afterAll 通过 API 清理）
- [ ] 3.4 实现 `test_story_04_hierarchy_tree.py`：点击 sector → 验证层级树过滤 → 点击 "全部" 恢复 → 验证高亮状态（无层级数据时 skip）
- [ ] 3.5 实现 `test_story_05_template_rebuild.py`：点击设置按钮 → 验证模板弹窗 → 修改 level → 点击保存 → 验证影响确认 → 确认重建 → 验证进度条
- [ ] 3.6 实现 `test_story_06_pending_changes.py`：验证 "待确认变更" badge → 点击打开面板 → 验证面板内容（无 pending 时 skip）

## 4. 验证

- [ ] 4.1 验证：`cd tests/acceptance && uv run pytest changes/unify-tag-hierarchy/ -v` 全部通过或合理 skip
- [ ] 4.2 验证：`cd tests/acceptance && uv run pytest changes/ -v` 所有变更验收测试可发现并运行
- [ ] 4.3 验证：单个 story 可独立运行 `uv run pytest changes/unify-tag-hierarchy/ -k "story_01" -v`
