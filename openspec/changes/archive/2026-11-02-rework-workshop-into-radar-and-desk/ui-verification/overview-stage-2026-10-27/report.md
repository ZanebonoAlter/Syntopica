# FB6 总览阶段：真实静态实页复验

## 范围与结论

仅总览获用户批准，实施者为 `zai-coding-cn/glm-5.3:max`。独立源码审查 Codex，截图视觉审查 `opencode-go/kimi-k3:high`。总览阶段已实现并部署到 `http://localhost:5100/tags`；矩阵/演进/归档/日报/关系编排产品源码未随本阶段改动。

此报告不替换根UI验收账本，不关闭旧R-11…R-16，不把整change或全仓类型检查称为通过。原根账本53 pass / 1 fail / 2未覆盖保留；本阶段S8证据单列。2560×1440为4K150%等效CSS视口，不是实物4K测试。

## 自动验证与类型欠账

- 类型消费者闭环目标命令：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2` → 5文件49测试通过。
- 最后UI互斥补丁：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → 1文件18测试通过；保留旧断言，增加首次失败右栏缺席/成功空集有右栏/已有旧列表刷新失败保留右栏。没有伪称一次命令50通过，也未为CSS重复全量单测。
- 受影响TS ESLint通过。现有ESLint配置不检查Vue，未宣称Vue lint通过，未新装工具；SFC通过generate编译及真实DOM验证。
- `cd front && pnpm exec nuxi typecheck`：本阶段诊断0，全局仍有 `SignalReportView.test.ts` 545/560/595–598/602的7处possibly undefined。已登记 `frontend-typecheck:app/features/tags/components/SignalReportView.test.ts`，不修改他域、不冒称全局通过。完整类型/单测日志在 `.pi/subagents/validation/overview-stage/`。

## 构建与发布

执行 `bash scripts/dev/deploy-frontend.sh --no-restart`，脚本内含generate/build，最终收口构建exit=0。没有额外重复执行pnpm build；实际UI发现窄屏CSS/错误右栏/宽屏cap后，针对修复重新铺产物。

原静态产物备份：`.pi/subagents/deploy-backups/overview-before-20261026T185523Z.tar.gz`。发布前核对：本阶段以外晚于原静态产物的前端脏文件只有BoardSourcePanel.test.ts（不入页面包），无并行重型构建/单测进程。共享semanticBoards.ts仅Radar尾块改变，其他API原修改保留。

后端未重启/重编；健康及tags shell HTTP 200。不SQL、不真实保存/新建/回填/合并/生成。验证浏览器均为独立命名会话，完成后显式关闭；浏览器注入仅影响其自身GET响应，不改服务端数据。

## S8 真实数据完整主链

操作方式：agent-browser打开静态 `/tags`，每次snapshot -i获取新ref后原生click，导航后读取URL/可见标题并截图；不是eval点击或用原型替生产。

| 节拍 | 实际结果与依据 |
| --- | --- |
| 总览真实取数 | 12活跃、4关注、0热点、1升温；日期来自overview。姓名/标题/数值非快照硬编码。 |
| 常温→展开完整横比 | 4行关注扩为12条真实story，1440无横溢。 |
| 无泳道有日报→实际读报 | GET证实影视作品board50413 lanes=0、report_date=2026-10-26；原生点击其日报标题到 `/tags/50413/2026-10-26`，可见“影视作品 / AI短剧是否“降智”引社会讨论 / 其他动态”。见 `zero-lane-report-arrival-1440.png`。 |
| 返回→小泳道矩阵 | 返回总览，点击Agent架构泳道原生按钮到 `/tags/1980?lane=90`，见 `mini-lane-arrival-1440.png`；再返回 `/tags`。不修改矩阵产品。 |
| 全局治理→匹配规则→取消 | 菜单作用域在总览；规则实际弹层中心elementFromPoint命中本体；取消回 `/tags`，没有保存。见 `global-rules-open-1440.png`。Esc须等待离场动画完成才断言；既有AppDialog role/焦点能力不扩改，未冒称全局无障碍合规。 |
| 唯一主创建入口→取消 | 打开真实新建弹窗，看到名称/描述输入；只取消，没有提交创建。组件mock另已覆盖创建成功重拉overview再去新板块的回程。 |

## 最终宽窄双主题几何

以下均真实:5100，不用fixture/原型洗绿正常态。最终图为`*-closure.png`；旧`*-final.png`和更早失败图继续保留，勿据旧文件名判断最新结果。

| 主题 / CSS视口 | document.scrollWidth | .radar宽 | 最终依据 |
| --- | --- | --- | --- |
| editorial 1440×900 | 1440 | 1440 | overview-editorial-1440-closure.png |
| editorial 1920×1080 | 1920 | 1920 | overview-editorial-1920-closure.png |
| editorial 2560×1440 | 2560 | 2560 | overview-editorial-2560-closure.png |
| editorial 390×844 | 390 | 390 | overview-editorial-390-closure.png |
| dark 375×667 | 375 | 375 | overview-dark-375-closure.png |
| dark 1440×900 | 1440 | 1440 | overview-dark-1440-closure.png |

375/390 summary“全局治理”的right=259.91，词组不拆，控件自然换行，没有靠overflow-x:hidden遮字段。R-20修后三列布局，R-21移除120rem私有限宽；未强拉行高。主题通过独立浏览器的应用既有`syntopica-theme`偏好+reload设置，不是硬改DOM颜色。

K3已对照原型1440/2560/375和对应实页三图，结论正常态OK with notes、无额外阻断；已知cap已移除，两个P2（标题孤字与“。·”）在同次小补丁收口，没有重设计用户批准方向。

## 故障/空集注入补充（明确不是业务量真库证据）

- 仅该浏览器 `network route **/api/semantic-boards/overview --abort`，reload模拟首次失败：标题“今日雷达”，error=true，业务空态=false，右栏=false，全文没有“今天还没有日报/暂未触发升温条件”。见 `overview-failure-injected-closure.png`。移除route后原生点击重试，真实4关注/1升温恢复。
- 成功空集：从真实HTTP响应克隆外壳（实际元数据success=true），仅把data.boards改为空数组，在该浏览器mock。error=false、业务空态=true、右栏=true、标题0关注/暂未升温，见 `overview-empty-injected-envelope-closure.png`。移除route并reload后真实数据恢复。
- 初次空集模拟误用缺HTTP外壳的raw DTO，客户端正确判失败；该设置错误截图保留为 `overview-empty-malformed-envelope-1440.png`，不当作空集通过证据，不为它放宽API协议。另一次eval引号错误改用base64 read-only eval后完成断言，没有冒称失败脚本已通过。
- 有旧列表刷新失败的保留语义由composable/组件单测覆盖；没有为了制造故障去真实创建/停服务/断网。未真实执行保存/生成/回填/合并/删除的写后效果，不据此宣称这些全产品能力已验收。

## 部署后影响与操作

只总览改报刊式横比、读报/泳道导航、常温展开及全局治理呈现。刷新总览即可，无需用户配置、重生成或迁移；后端统计及JSON协议不变。旧字段按原缺省策略展示，无锚日不伪造截止日期，无活跃泳道但已有日报仍可读。

矩阵仍为旧产品，下一步只设计其自由时间/自然高度/来源多图原型；不代表该区域已批准实现。harden-ui-delivery-acceptance门禁工具交付层与其它剩余任务继续保留，不归档、不提交整change。
