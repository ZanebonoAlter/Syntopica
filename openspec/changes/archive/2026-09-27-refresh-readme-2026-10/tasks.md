# Tasks — refresh-readme-2026-10

## 1. 截图与资产准备

- [x] 1.1 通过 `bash scripts/dev/start-dev.sh` 起本地栈并确认演示 seed 数据就绪（auto-sync-demo-seed），静态托管入口 `:5100` 可访问，为截图提供实机状态
- [x] 1.2 新截两张关键图：板块信号解读工作台（数据增强主视图）、泳道动态视图；存入 `img/readme/`，验证文件存在且非空
- [x] 1.3 收敛 `img/`：保留仍反映现状的旧图迁入 `img/readme/`，移除不再被引用的资产（`1.3.3/`、`1.4.0/`、`product-video-v2/` 等旧目录），验证 `git status` 下仅 README 与 img 变更
  - 执行注：`product-video-v2/` 与 `img/image-topic.png` 仍被 `docs/experience/product-video-v1.md`、`mimo-tts-and-jianying.md` 引用，按本任务限定词「不再被引用的资产」保留原位（避免 docs 改动破坏「仅 README 与 img 变更」验证）；其中 3 帧复制入 `img/readme/` 供 README 引用，其余旧目录/根图全删

## 2. 内容迁移与 README 重写

- [x] 2.1 核对 `docs/reference/deployment.md`：将 llama.cpp 完整命令行示例与显存参考表补入（若无对应章节），验证该文档新增节存在且命令可读
- [x] 2.2 按 design.md D1 骨架重写 `README.md` 全文（11 节、约 320 行）：旅程 6 节每节 1 图、功能全景按 map.md 业务域补齐（信号报告/简报调查/跨版块关系/追问/泳道动态/旁注/归档/图片代理/来源质量；FinGenius 不写；研究数据源一句话 alpha）、对比与适合谁合并、llama.cpp 示例外链 deployment.md
  - 执行注：产品闭环 ASCII 图先改 diagram-design 品牌皮肤架构图（product-loop），2026-09-27 用户二次指令后改为**用户操作驱动的顺序图**（img/readme/llm-spend.html → llm-spend.png 1800×1384；叙事口径回溯 ssp-article 讨论红线，见 design.md D1）；README 对应节改为「运转方式：LLM 花在哪里」
- [x] 2.3 事实修正内嵌于 2.2：端口 5100 ×3、死链 2 处移除、「兜底」错别字、`expand-board.png` alt、`deploy/same-origin/` 目录树描述与 Go 单进程同域部署形态，验证方式见尾节验证命令

## 3. 测试

纯文档 change：豁免代码测试与 test-cases.md（test-design JIT 规则）。无行为变化，无测试资产继承。

## 4. 文档

<!-- doc-impact: deployment -->
<!-- README 为仓库级门面文档不属 8 域；docs/reference/deployment.md 扩写 llama.cpp/显存表节，无业务链路变更 -->

- [x] 4.1 README 重写完成（任务 2.2 即本 change 的文档交付物本体）
- [x] 4.2 deployment.md 扩写 llama.cpp/显存表节（任务 2.1，属文档改动而非 flow 变更）

## 5. 验证

- [x] 5.1 grep 一致性校验全绿：`grep -nE "localhost:5000|data-flow\.md|topic-graph\.md|兑底" README.md` 零命中；`grep -c "5100" README.md` ≥ 2
  - 留痕：`grep -nE ...` exit=1（零命中）；`grep -c "5100" README.md` → `5`（≥2 ✓）
- [x] 5.2 README 内全部相对链接与图片路径存在性校验：逐个提取 `](path)` 与 `src="path"` 引用并 `test -e`，零缺失（脚本式走查，命令与输出留痕于本节）
  - 留痕：python 脚本提取 `\]\((path)\)` + `src="(path)"` 去重后 20 条引用，逐条 `os.path.exists` 检查，输出 `本地缺失: 0`（含 9 张 `img/readme/` 图 + favicon + 7 个 docs 链接 + LICENSE）
- [x] 5.3 行数目标核对：`wc -l README.md` 落在 290-360 区间（design 目标 320 ±10%）
  - 留痕：`wc -l README.md` → `326`（ASCII 闭环图改图片后余量充足 ✓）
- [x] 5.4 人工：浏览器打开 GitHub 渲染态（或本地 markdown 预览）通读一遍，确认章节归属无重复、截图显示正常、产品调性未丢（2026-09-27 用户确认通过，含顺序图返工版）
  - 机器预检留痕（2026-09-27，glm-5.3-flash 浏览器渲染走查）：本地 markdown 渲染后 11/11 图片全部加载（naturalWidth>0）；11 节结构齐全顺序正确；理念/功能清单/对比各只出现一处；两张新截图清晰；调性克制无营销腔。首检发现 product-loop.png 截残（元素截图被视口裁剪）→ 独立页零偏移重渲染后复检 9 节点/角标/反馈箭头/图例全部 ✓。待用户人工终审后勾选
