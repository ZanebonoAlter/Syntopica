## 1. 配置源（仓库）

- [x] 1.1 新增 `config/dsh/presets/energy-research/preset.yml`（name=能源研究 / 描述含「仅网页取材、专业数据接口待接入」/ order: 20），静态结构校验通过（5.1 命令实跑 ALL PASS）
- [x] 1.2 新增 `config/dsh/presets/energy-research/agent.cordis.yml`：persona（中文研究纪律）+ tool-web（fetch/searchTimeoutMs）+ compaction 组（isolate compaction/toolResultPruner，8192/4096/1024）+ tool-ask-user；无 shell/fs/editor/delegation/skills/todo/MCP 行，包名 6/6 在 npx 缓存核对到目录（5.2）
- [x] 1.3 仓库源 SHA256：preset.yml `afa39270d713a69de69c7e13c43528cb3348c6abe4fcc568b2f019a23c6ff4c7`、agent.cordis.yml `a3fd3073db269bf5e50d7bcecd112dd552f6d35876ce47ecd74706147d9839bb`

## 2. live 部署（外部，仅两文件）

- [x] 2.1 部署前快照受保护配置哈希：`/tmp/dsh-protected-before.txt`（7 文件 sha256sum 输出：`.anonymous-user-id`/`.credentials.yaml`/`settings.yaml`/`profiles/web/` 4 文件；只哈希未读内容；快照为部署轮子线程当轮输出）
- [x] 2.2 部署前确认 `<dshHome>/.agent-presets/` 不存在（无并发覆盖风险）；复制两文件后 `cmp` 逐字节一致，live SHA256 与源相等（值见 5.5）；live 目录仅此 2 文件
- [x] 2.3 部署后快照 `/tmp/dsh-protected-after.txt` 与 before `diff` 零差异；收尾轮仅 diff 两快照复核，未再读取受保护文件本体（部署前基线无法由第三方独立重建，以子线程当轮输出+快照文件为证）

## 3. 测试

本 change 无应用代码改动（front/、backend-go/ 零触碰），豁免 go/pnpm 测试；以静态结构断言 + 哈希对比代替（第 5 节，命令可复跑）。UI 运行时验收受阻项如实保留未勾（5.8）。两轮均未触发任何模型请求或网页搜索。

## 4. 文档

<!-- doc-impact: configuration -->

- [x] 4.1 `docs/reference/configuration.md` 增补「dsh 能源研究本地预设」节（L338 起）：仓库源与 live 路径、新建会话选预设操作、旧会话/默认值保留、删除用户预设目录的回退语义（停止向新会话提供；已运行会话及历史不删除/撤销）、当前无专业数据源与整场硬预算、版本前提 dsh 0.1.2-rc.1；收尾轮修正「DeepSeek Shell」→「DeepSeek Harness」

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 新预设被发现 | 人工：5.8 opencli 运行时验证（roster 可见、可选、非 broken） |
| 默认与旧会话保持不变 | 人工：5.3 部署前后哈希对比；全程无 settings/凭据/默认值写操作 |
| 工具目录只含预期研究工具 | 人工：5.1 结构断言（静态配置面；**运行时工具目录未执行验证**，不冒充） |
| 无虚假专业数据源 | 人工：5.4 grep |
| 源与 live 一致 | 人工：5.5 sha256 对比 |
| 部署零副作用 | 人工：5.3 |
| persona 含纪律要点 | 人工：5.1 关键词断言（14 词全命中） |
| 文档可指导部署与回退 | 人工：5.6 grep |

- [x] 5.1 静态结构断言：`node openspec/changes/configure-dsh-energy-research/verify-structure.mjs`（Node v24.14.0 + npx 缓存自带 js-yaml，零新增依赖；断言清单见脚本注释）→ 输出 `ASSERTIONS: ALL PASS`，rc=0（本轮实跑通过）
- [x] 5.2 包名存在性：`ls -d` 六个 `@deepseek-ai/dsh-*` 目录于 `/mnt/c/Users/Admin/AppData/Local/npm-cache/_npx/1e7f6d9597241db0/node_modules/` → 6/6 OK
- [x] 5.3 受保护配置零副作用：`diff /tmp/dsh-protected-before.txt /tmp/dsh-protected-after.txt` → 无输出零差异（7 文件部署前后 sha256sum 逐一相等；证据为部署轮快照，收尾轮未重读文件本体）
- [x] 5.4 内容红线 grep（实跑结果）：`grep -niE "eia|jodi|steo|mcp" config/dsh/presets/energy-research/*.yml` → 仅 2 处命中：agent.cordis.yml L6 注释、L26 persona「未接入」声明（preset.yml 零命中）；`grep -nE "https?://"` → 零命中（rc=1）；`grep -nE ":[0-9]{4,}"` → 零命中（无虚构端口）；persona 纪律关键词 14 词经 5.1 脚本逐词断言命中
- [x] 5.5 源/live 一致（收尾轮复跑）：`sha256sum` 仓库源与 `/mnt/c/Users/Admin/.dsh/.agent-presets/energy-research/` 两文件 → preset.yml 源=live `afa39270d713a69de69c7e13c43528cb3348c6abe4fcc568b2f019a23c6ff4c7`；agent.cordis.yml 源=live `a3fd3073db269bf5e50d7bcecd112dd552f6d35876ce47ecd74706147d9839bb`
- [x] 5.6 文档节存在：`grep -n "能源研究" docs/reference/configuration.md` → 命中 L338 新节标题及节内多处；节内含「回退」「未接入」关键词（收尾修正后复跑仍命中）
- [x] 5.7 openspec 校验（收尾轮复跑）：`openspec validate configure-dsh-energy-research` → valid；`bash scripts/doc-impact.sh verify openspec/changes/configure-dsh-energy-research` → 退出码 0（声明 configuration，启发式 ownership 轨）；`bash scripts/check-standards.sh --change configure-dsh-energy-research` → 本 change 相关段（A-D/F/G）零失败，其中 F 段 doc-impact 通过、G 段死链零失败；**E 段存在与本 change 无关的既有欠账**：归档 `2026-09-06-split-database-docs-by-domain` 未被任何 `docs/reference/flow/*.md` 变更溯源表引用（属他人已归档 change 的文件，本 change 不修改，不宣称全局门禁全绿）
- [x] 5.8 UI 抽查（运行时验证通过）：opencli 会话 cfgv 访问 `http://127.0.0.1:3080/`（127.0.0.1 域下真实 Chrome 既有登录态有效；此前 `localhost` 域 401 系 cookie 域差异，非预设问题），页面 title「DeepSeek Harness」。新会话页 Agent 预设按钮（title=「即将开始的这个会话所用的 Agent 预设」）当前显示「能源研究」；eval 只读展开选择器（读后即关）得 roster 全量五项：标准模式 / PTC 模式 / 极简模式 / 创造模式 / 能源研究——「能源研究」条目描述与 `preset.yml` 逐字一致（「原油供需研究助手：证据优先，先查事实再解释；当前仅网页检索与抓取材料，专业数据接口待接入；无 Shell、文件编辑与委派能力。」），无 broken/错误标记，内置「标准模式」仍在。全程零 prompt、零模型/工具调用、未创建会话、未改任何设置；截图因浏览器动作预算（3 次：open/state/eval）省略，以 menu 文本 + state 输出为证据。验证范围声明：roster 发现/可选已验；预设内工具目录的运行时执行未测（不在本 change 验收范围） |
