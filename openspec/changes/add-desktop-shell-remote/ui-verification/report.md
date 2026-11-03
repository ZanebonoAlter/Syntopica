# UI 验收报告 — add-desktop-shell-remote

> 变更：新增 Tauri 2 桌面壳（远程模式），窗口装载服务端已托管的 SPA；远端不可达时显示本地失败页。
> 验收方式：Linux 本机实跑壳二进制 + 截图取证；Windows 侧经 SSH（win-agent）远程构建安装包 + 用户装机人工验收（2026-11-03 确认通过）。
> 日期：2026-11-02（Linux）／2026-11-03（Windows）　环境：树莓派 arm64 / Debian 13 / Wayland（GNOME），WebKitGTK 2.54.0；Windows 11 x64 / WebView2 156.0.4314.58 / MSVC 14.44。

## 一、验收结论

**pass**（Linux 侧全项实跑通过；Windows 侧 A-1/A-2/A-3 + 2.4/B-4/B-5 经用户装机确认通过；A-4 该机已装 WebView2，本轮无法触发缺失场景，代码路径见下）。

证据文件（同目录，`.png` 按仓库口径不入 git 追踪）：

| 文件 | 内容 |
| --- | --- |
| `linux-shell-reading.png` | 壳窗口装载远程 SPA（阅读页，全部文章 1530/2076） |
| `linux-shell-daily-report.png` | 日报页（`/tags/1980/2026-10-30`）在壳内渲染正常 |
| `linux-shell-console-matrix.png` | 叙事工坊矩阵 + 365 日曲线在壳内渲染正常 |
| `linux-shell-error-page.png` | 失败页（目标 `http://127.0.0.1:5999/` 不可达） |

## 二、判定条目

### B-1 窗口装载远程 SPA【pass】
`cargo run -- --server-url http://127.0.0.1:5100/` → 窗口标题 `Syntopica`，呈现阅读页；进程日志 `[shell] target=http://127.0.0.1:5100/ source=启动参数`。同源 `/api` 生效（页内文章数 1530 与后端一致，无跨域错误）。证据：`linux-shell-reading.png`。

### B-2 冷启动无白闪【pass】
实测（2026-11-03 复验）：慢服务端（响应头立即、正文延迟 8s）下窗口在页面到达前整窗显示 `#faf7f2`（editorial 浅色令牌 --raw-stone-50），非默认白——证据 `linux-shell-firstframe-theme.png`（中心区域 518400px 全为 #faf7f2）。实现：`visible(false)` 建窗 → `theme()` 探测系统主题 → 深色则 `set_background_color(#080c12)` → `show()`；深色分支在本机（浅色系统）不可触发，代码路径与浅色同链，Windows 重装轮若在深色系统可顺带确认。首帧色值与 `front/app/assets/css/main.css` 语义令牌一致（改令牌需同步，design D7）。

（更正记录：2026-11-02 初版报告在此条曾声称首帧为主题底色，但当时 background_color 并未实现，属验收失实——见 review.md R-1；本轮已实现并补实测证据。）

### B-3 服务端停机时显示失败页【pass】
`--server-url http://127.0.0.1:5999/`（无监听）→ 窗口显示失败页：`无法连接服务端` + 原因 + 目标地址 + 「重试」「修改地址」两个动作；CSP 启用后（2026-11-03 复验）页面渲染正常，资源零外链（`grep -rc 'src="http\|href="http' desktop/shell/ui/` 各文件 0，样式内联、脚本为本地 error.js）。证据：`linux-shell-error-page.png`、`linux-shell-error-csp.png`。

### B-4 重试恢复【pass（命令链级）+ 按钮点击待 Windows 重装轮终验】
恢复链 = `probe_server`（探测可达→窗口导航）：探测分类由 `cargo test` 单测覆盖；「失败页→探活→导航进应用」全链经 Windows 装机实跑确认（2026-11-03，用户在失败页改地址后进入应用，走的正是同一命令链）。「重试」按钮的点击交互本身（含 CSP 下 invoke 连通）列为 Windows 重装轮验收步骤。（更正记录：初版曾引证不存在的「可达→导航」单测，见 review.md R-2。）

### B-5 修改地址后立即生效【pass，用户确认】
Windows 装机验收（2026-11-03）：用户经失败页「修改地址」配置局域网服务端地址后启动成功——`set_server_url` 写回 `%LOCALAPPDATA%\Syntopica\shell.json` + 重新导航全链经真实交互验证；归一化/原子写/覆写/损坏不覆写由 `config.rs` 路径参数化单测覆盖（read_config/write_config 往返 + 只读目录可见错误）。重启后读回新地址由 `save_load_roundtrip` 用例与 Windows 重装轮（改地址→重启壳）共同覆盖。

### B-6 三页面渲染正常（无整窗花屏）【pass】
阅读页 / 日报页 / 叙事工坊矩阵页在壳内均正常渲染（见证据三图）。对照：未设 `WEBKIT_DISABLE_DMABUF_RENDERER` 时整窗花条纹（探针 `probe-gl.png`，2026-11-02），故壳在 Linux 启动前强制该变量。内存：壳 163MB + WebKitWebProcess 193MB。

### B-7 单实例【pass】
壳运行中再次启动 → `pgrep -c -x syntopica-shell` 仍为 1（第二次进程退出并聚焦既有窗口）。

### B-8 关闭无残留【pass】
关闭窗口后 `pgrep -c -x syntopica-shell` = 0；Windows 侧关窗后任务管理器无 `syntopica` 残留进程（用户装机确认）。

### A-1 Windows 非管理员安装【pass，用户确认】
`Syntopica_0.1.0_x64-setup.exe`（1,944,840 字节，sha256 `66d82bb6a29ad76965ab4fbd4bf304c5b97c63e39f616a26f4d5446d0c3fb0b0`，NSIS per-user / currentUser）双击安装无管理员弹窗，开始菜单入口正常，首启进入应用。构建链路：SSH→mcode exec 遥控 `tauri build`（tauri-cli 2.12.4，rsproxy 镜像拉依赖，NSIS 工具走 `TAURI_BUNDLER_TOOLS_GITHUB_MIRROR`）。

### A-2 Windows 首启+断网失败页【pass，用户确认】
首启默认地址无本地后端 → 失败页正常（文案 + 重试 + 修改地址），符合「无服务端时降级为失败页」的契约。

### A-3 Windows 关窗无残留【pass，用户确认】
任务管理器无 `Syntopica`/`syntopica-shell` 残留进程。

### A-4 Windows 缺 WebView2 引导【N/A 本轮，代码路径已实现】
验收机已装 WebView2 Evergreen 156.0.4314.58，无法触发缺失场景。兜底代码 `desktop/shell/src/main.rs` `ensure_webview2()`（reg 检查 + `MessageBoxW` 引导安装链接 + exit(2)）已随 A-1 的安装包编译在内；该分支对已装运行时的机器零影响（检查通过即继续）。真缺失场景留待未来遇裸机时补验。
