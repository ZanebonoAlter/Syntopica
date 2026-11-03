# desktop/

Syntopica 桌面装法（Tauri 壳）的工程家。

- **一份代码两种装法**：产品代码共享 `backend-go/` 与 `front/`，本目录只放桌面壳特有的东西。
- **当前只实现远程模式**（change `add-desktop-shell-remote`）：窗口装载服务端已托管的 SPA，同源 `/api`，不启动本地后端。
- `shell/`：Tauri 2 应用（Rust）。
- `lab/`：迭代 0 的验证区——sidecar 进程协议（0-3）的模拟壳与时序实测脚本，可复跑。
- 运行时数据目录不在仓库：Linux/macOS `~/.syntopica/`、Windows `%LOCALAPPDATA%\Syntopica\`（见 `docs/srs/syntopica-client/iter0-findings.md`）。

## 远程壳（remote mode）

窗口导航到服务端地址，页面由后端同源托管，**前端零改动**（`NUXT_PUBLIC_API_BASE=/api` 已是同源相对）。

地址优先级（`src/config.rs`）：

```
--server-url <url>  >  SYNTOPICA_SERVER_URL  >  本地 shell.json  >  http://127.0.0.1:5100/
```

本地配置路径：Windows `%LOCALAPPDATA%\Syntopica\shell.json`；Linux/macOS `~/.syntopica/shell.json`（由失败页「修改地址」写入，原子写）。

### 开发

```bash
cd desktop/shell
cargo run -- --server-url http://127.0.0.1:5100/     # 需要服务端在跑
cargo test                                            # 地址解析 / 探测分类 / 编码
```

Linux 上题目注意：编译产物别放 `/tmp`（这台机器 `/tmp` 是 tmpfs，2~3GB 的 target 会把它撑爆）——用
`CARGO_TARGET_DIR=$HOME/.cache/syntopica/cargo-target` 或默认的 `desktop/shell/target/`（已 gitignore）。

### 打包（Windows）

```powershell
cd desktop\shell
cargo tauri build      # 或 pnpm tauri build
# 产物：target\release\bundle\nsis\Syntopica_<version>_x64-setup.exe（per-user，免管理员）
```

前置：Rust（默认 MSVC 目标）+ VS Build Tools「使用 C++ 的桌面开发」+ WebView2 Runtime（Win11 自带；缺失时走 Evergreen 引导）。

> 官方口径（Tauri 2 文档）：`.msi` 只能在 Windows 生成；NSIS 可在 Linux/macOS 交叉编译但「不被充分测试、只应作为最后手段」。本仓库约定：**开发与渲染验证在 Linux，出包在 Windows**。

### 平台差异

| 平台 | 渲染 | 备注 |
| --- | --- | --- |
| Windows | WebView2（Chromium） | 无额外开关 |
| Linux | WebKitGTK 4.1 | 启动时强制 `WEBKIT_DISABLE_DMABUF_RENDERER=1`（壳内置） |
| macOS | WKWebView | 未验证 |

**为什么 Linux 要关 DMABUF**：2026-11-02 在 Pi 5（Debian 13 / Mesa / WebKitGTK 2.54）实测，默认 GL 路径整窗花条纹；关闭后阅读页 / 日报页 / 叙事工坊矩阵页均正常，62fps、p95 17ms（对照 `LIBGL_ALWAYS_SOFTWARE=1` 也能正常但更慢，60.2fps / p95 21ms）。内存基线：壳 157MB + WebKitWebProcess 204~218MB + NetworkProcess 75~86MB。

### 已知限制

- **断网不可用**：远程模式没有本地库，服务端不可达时只能显示失败页（重试 / 改地址）。
- **会话内服务端失效无壳内兑底**：启动探测通过后、会话期间服务端挂掉（加载中断、TLS 损坏）时显示 WebView 原生错误页，无重试入口——重开应用即回失败页流程；会话内导航失败监听留给后续 change。
- **启动探测同步阻塞至多 5 秒**：目标地址网络黑洞（不拒连只丢包）时窗口最多 5 秒后才出现；这 5 秒内二次启动不会聚焦已有窗口（窗口尚未创建），第二进程静默退出。
- **明文 HTTP 仅限可信网络**：`http://` 地址适合本机/家庭局域网；公网需前置 TLS 反代。
- **`https://` 只做 TCP 可达性检查**：启动探测不会做 TLS 握手（壳不需要，明文 HTTP 才是文档化用法），所以 https 下看不到 4xx/5xx 的分类，只区分「端口可达 / 不可达」；也探测不出「端口可达但 TLS 损坏」的场景（导航后由 WebView 原生错误页呈现）。
- **非安全上下文的 Web API**：在 `http://<局域网IP>` 下 `navigator.clipboard`、`crypto.subtle` 等不可用（`http://localhost` 例外）；当前前端只用 `crypto.getRandomValues`（不受限），新增受限 API 前需评估。
- 未实现：sidecar 本地后端、embedded PG、素材包同步、自动更新、托盘常驻、代码签名。
