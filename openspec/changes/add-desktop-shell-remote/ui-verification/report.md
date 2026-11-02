# UI 验收报告 — add-desktop-shell-remote

> 变更：新增 Tauri 2 桌面壳（远程模式），窗口装载服务端已托管的 SPA；远端不可达时显示本地失败页。
> 验收方式：Linux 本机实跑壳二进制 + 截图取证；Windows 装机项因当前无目标机器标记为「未覆盖」，待 Windows 机器上补。
> 日期：2026-11-02　环境：树莓派 arm64 / Debian 13 / Wayland（GNOME），WebKitGTK 2.54.0，服务端 `http://127.0.0.1:5100`（真实库）。

## 一、验收结论

**部分通过**：Linux 侧全项 pass（窗口装载 / 首帧 / 失败页 / 单实例 / 三页面渲染）；Windows 侧 A-1~A-4 为 **未覆盖**（无 Windows 机器，需在目标机上补跑）。

证据文件（同目录，`.png` 已按仓库口径不入 git 追踪）：

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
窗口首帧为 editorial 主题底色，随后由 SPA 自身首屏模板接管；截图中未出现白屏帧。说明：首帧色值与 `front/app/assets/css/main.css` 语义令牌一致（本 change 内联复制，改令牌需同步）。

### B-3 服务端停机时显示失败页【pass】
`--server-url http://127.0.0.1:5999/`（无监听）→ 窗口显示失败页：`无法连接服务端` + 原因 `服务端未启动，或该地址不可达（当前地址来自启动参数）` + 目标地址 + 「重试」「修改地址」两个动作；页面无外链资源（`grep -c "http" desktop/shell/ui/error.html` = 0，全部内联）。证据：`linux-shell-error-page.png`。

### B-4 重试恢复【未覆盖 — 需人工点击】
重试逻辑由 `probe_server` 命令承载（单测覆盖「可达 → 导航」「不可达 → 返回原因」的判定链，`cargo test` 19 项通过），但**按钮点击后的实际跳转**需要在有交互能力的环境点一次；本次 Linux 冒烟未做点击注入，标记未覆盖，Windows 装机验收时补。

### B-5 修改地址并持久化【未覆盖 — 需人工点击】
`set_server_url` 的归一化 + 原子写入 + 探测链由单测覆盖（`config::tests` 中优先级/归一化/损坏回退），点击链路同上待补。写入目标：Linux `~/.syntopica/shell.json`、Windows `%LOCALAPPDATA%\Syntopica\shell.json`。

### B-6 三页面渲染正常（无整窗花屏）【pass】
阅读页 / 日报页 / 叙事工坊矩阵页在壳内均正常渲染（见证据三图）。对照：未设 `WEBKIT_DISABLE_DMABUF_RENDERER` 时整窗花条纹（探针 `probe-gl.png`，2026-11-02），故壳在 Linux 启动前强制该变量。内存：壳 163MB + WebKitWebProcess 193MB。

### B-7 单实例【pass】
壳运行中再次启动 → `pgrep -c -x syntopica-shell` 仍为 1（第二次进程退出并聚焦既有窗口）。

### B-8 关闭无残留【pass】
关闭窗口后 `pgrep -c -x syntopica-shell` = 0。

### A-1 Windows 非管理员安装【未覆盖】
原因：当前无 Windows 目标机（出包与装机按 design D8 在 Windows 本机执行）。待补：安装包双击、开始菜单入口、首启进入应用。

### A-2 Windows 断网重试恢复【未覆盖】
同上。

### A-3 Windows 关窗无残留【未覆盖】
同上（任务管理器核对 `Syntopica.exe`/`syntopica-shell.exe` 消失）。

### A-4 Windows 缺 WebView2 引导【未覆盖】
同上（需在未装 WebView2 的机器或手动卸载后验证引导出现）。
