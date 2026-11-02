# 实页随手取证（2026-10-25）

## F1 暂定 High：有日报但无 active 泳道，控制台没有日报入口
- 已实测打开 `/tags/50413`，等待「暂无活跃泳道」。03 文件名带1440但PNG/DOM实测为1280×633，不能计作1440。另04真1920×1080及09真375×667同样复现。工具 snapshot 仅有矩阵/归档/时间窗口/新建泳道，无今日日报/最新日报；真实页面标题为影视作品。
- 已保存 `03-50413-1440.png`、`03-50413-1440.snapshot.txt`、`03-50413-dom.json`。
- 主线程交叉证据：has_reports=true、lanes=[]，report 1237 于 10-24 含 1 分区 2 线索。下一步直接打开已知真实日期核验，不调写接口。
- 已复核代码锚：useBoardConsole.ts:250-257 仅从 active lanes 推 latestDataDate；BoardConsoleShell.vue:457 按 latestDataDate 决定报告入口。
- `04-50413-report-1920.snapshot.txt` 及同名PNG证实直达10-24有1分区2线索。

## 已确认的其他观察
- `10-50413-article-drawer-375.png`：归档行只打开匹配证据，唯一阅读出口是外站「打开原文」，未提供旧 ArticlePreviewModal 的站内全文能力。
- `06-1974-topic-evolution-1920.png`：泳道趋势、当期关键事件按钮与 mini 生命线仍在；不能称演进全丢，也不能把已批准删除的右栏本身判错。
- `06-1974-topic-evolution.snapshot.txt` 有「在侦探墙打开完整生命线」，内部话题管理未打开；不声称话题CRUD全站不可达。
- `10-50413-archive-7days.snapshot.txt` 记录10-21同日09:02先于20:19、23:20；源码useBoardArchive.ts:114-119确实仅按日+id排序，非发布时间。
- 阅读台仅日期导航，没有原DailyReportSidebar中的版块切换。手机实际可经「矩阵→叙事工坊→常温→影视」切版块，不能称手机完全不能切版块。

## 终止与证据边界
- 第二次阻塞：`scrollintoview '.ar-day:nth-child(5)'` 返回Element not found。bash EXIT finally已执行唯一会话close，见session-finally-close.log。
- `11-50413-evidence-only-1440.png`为视口切换后的白屏过渡截图，不作产品白屏证据；没有再启动浏览器重试。
- 03真1280已纠正；50413的1440同场景未覆盖。1974的02真1440、06真1920、07真375可用。
- 历史section→归档日期传递仅静态确认未传日期，未完成实页复现；编排采纳0、研究/治理弹窗、报告历史管理及dark未完成。
