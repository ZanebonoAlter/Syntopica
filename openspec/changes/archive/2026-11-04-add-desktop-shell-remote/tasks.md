## 1. 工程骨架

- [x] 1.1 新建 `desktop/shell/` Tauri 2 工程（`Cargo.toml`、`build.rs`、`src/main.rs`、`tauri.conf.json`、`icons/icon.png`、`ui/error.html` 占位），验证：`cd desktop/shell && cargo build` 退出码 0
- [x] 1.2 窗口创建（`main.rs` 运行时建窗：默认 1100×720、最小 900×600、标题 `Syntopica`；`background_color` 取主题令牌、`visible(false)` + 系统主题探测后再 `show()`，保证深色系统首帧非白闪），验证：`grep -c 'background_color\|THEME_BG' desktop/shell/src/main.rs` ≥ 3 且慢服务端首帧截图主色为 `#faf7f2`（见 ui-verification/linux-shell-firstframe-theme.png）

## 2. 地址装载与失败兜底

- [x] 2.1 实现目标地址解析（启动参数 `--server-url` > 环境变量 `SYNTOPICA_SERVER_URL` > 本地配置 > 默认 `http://127.0.0.1:5100/`），配置目录按平台（Windows `%LOCALAPPDATA%\Syntopica\`、Unix `~/.syntopica/`），验证：`cargo test` 中四级优先级用例（arg 两种形式/env 压 file/file/default）+ `read_config` 不存在返回 Ok(None)/损坏返回 Err 且不覆写 + 只读目录写返回可见错误，全部通过
- [x] 2.2 导航失败（连接拒绝/超时/非成功状态）捕获并加载 `ui/error.html`，验证：`cargo test` 中失败判定纯函数用例通过；手工停服务端启动壳 → 窗口内可见失败页
- [x] 2.3 `ui/error.html` 实现（原因说明 + 「重试」+ 「修改地址」，样式内联 + inline SVG，脚本为本地 `error.js`（CSP 下 `script-src 'self'`），零外链资源），验证：断网状态下打开该文件渲染完整（`grep -rc 'src="http\|href="http' desktop/shell/ui/` 各文件均 0 表示无外链资源）
- [x] 2.4 「修改地址」写回本地配置并重新导航，验证：`cargo test` 中 `save_load_roundtrip_via_parameterized_paths`（写→读一致、覆写生效、无残留 tmp）通过；Windows 装机实跑：失败页改地址 → 进入应用（用户确认 2026-11-03）

## 3. 平台渲染适配与单实例

- [x] 3.1 Linux 目标在创建窗口前设置 `WEBKIT_DISABLE_DMABUF_RENDERER=1`（`#[cfg(target_os = "linux")]`），Windows/macOS 不设置任何渲染开关，验证：`cargo build` 通过 + 代码检查 `grep -n "WEBKIT_DISABLE_DMABUF_RENDERER" desktop/shell/src/main.rs`
- [x] 3.2 接入单实例（重复启动聚焦已有窗口），验证：Linux 上连续启动两次 → 只存在一个窗口/一个进程组；`pgrep -c -x syntopica-shell`=1

## 4. Windows 分发

- [x] 4.1 `tauri.conf.json` 配置 bundle：`targets=["nsis"]`、`windows.nsis.installMode="currentUser"`、`webviewInstallMode=downloadBootstrapper`，验证：`python3 -c "import json;c=json.load(open('desktop/shell/tauri.conf.json'));print(c['bundle']['targets'], c['bundle']['windows']['nsis']['installMode'])"` 输出 `['nsis'] currentUser`
- [x] 4.2 WebView2 缺失时给出安装引导（不白窗/不静默失败），验证：见 test-cases.md「Windows 缺 WebView2」人工项；Linux 侧以代码检查确认未把引导逻辑放到非 Windows 分支
- [x] 4.3 在 Windows 目标机执行 `pnpm tauri build`（或 `cargo tauri build`）产出 NSIS 安装包，验证：产物存在且文件名含版本号（`dir /b src-tauri\target\release\bundle\nsis`）

## 5. 开发验证脚本与说明

- [x] 5.1 在 `desktop/shell/` 提供 `run-dev.md` 片段或 README 中的开发命令（Linux 直连 `cargo run -- --server-url http://127.0.0.1:5100`），验证：命令可直接复制执行且窗口成功打开
- [x] 5.2 记录探针基线（WebKit 花屏结论、`WEBKIT_DISABLE_DMABUF_RENDERER` 实测帧率/内存）到 `desktop/README.md`，验证：`grep -n "WEBKIT_DISABLE_DMABUF_RENDERER" desktop/README.md` 有命中

## 6. 测试

- [x] 6.1 按 `test-cases.md` 主链路表逐行执行，人工项结果记入 `ui-verification/report.md`，验证：报告内每行状态为 pass 或有原因标注
- [x] 6.2 地址解析与失败判定的纯函数用例落地（`desktop/shell/src/config.rs` / `nav.rs` 内 `#[cfg(test)]`），验证：`cd desktop/shell && cargo test` 全绿
- [x] 6.3 变体走查（空地址、畸形 URL、配置文件损坏、超长地址、重复启动、并发启动）逐项有结论，验证：`test-cases.md`「变体走查」节无空白行
- [x] 6.4 效果核对：Windows 真实安装 + 首次启动 + 断网恢复三场景的人工证据，验证：`ui-verification/report.md` 内 A-1/A-2/A-3 三行存在且带截图或命令输出
<!-- doc-impact: deployment, configuration -->

## 7. 文档

- [x] 7.1 `desktop/README.md` 补「远程壳」章节：定位、构建/打包命令、地址配置、平台差异（Linux DMABUF / Windows WebView2）、已知限制（断网不可用、明文 HTTP 仅限可信网络），验证：`grep -c "远程壳" desktop/README.md` ≥1
- [x] 7.2 `docs/reference/deployment.md` 增加「桌面壳（远程模式）」小节，含安装包获取、首次配置地址、卸载回滚，验证：`grep -n "桌面壳（远程模式）" docs/reference/deployment.md` 有命中
- [x] 7.3 `docs/reference/configuration.md` 增加壳的地址配置项（`--server-url` / `SYNTOPICA_SERVER_URL` / 本地配置文件路径），验证：`grep -n "SYNTOPICA_SERVER_URL" docs/reference/configuration.md` 有命中
- 无 flow 影响：本 change 只新增桌面壳（Tauri 远程装载，`desktop/shell/`），不改任何业务 flow 的生成/编排流程（`backend-go/`、`front/` 业务链路零改动），按《开发执行规范》§12.2 豁免 flow 变更溯源

## 8. 验证

- [x] 8.1 `cd desktop/shell && cargo test && cargo build` → 退出码 0，输出 `test result: ok`
- [x] 8.2 `cd desktop/shell && cargo clippy -- -D warnings` → 退出码 0（若本机未安装 clippy，先 `rustup component add clippy`）
- [x] 8.3 `openspec validate add-desktop-shell-remote --strict` → 退出码 0
- [x] 8.4 `bash scripts/harness/scenario-trace.sh openspec/changes/add-desktop-shell-remote && bash scripts/harness/doc-impact.sh verify openspec/changes/add-desktop-shell-remote && bash scripts/harness/check-standards.sh --change add-desktop-shell-remote` → 三条命令退出码均为 0
- [x] 8.5 Linux 手工冒烟：`cargo run -- --server-url http://127.0.0.1:5100` → 窗口出现且阅读页/日报页/矩阵页三个页面渲染正常（无整窗条纹），失败页在服务端停止后出现（重试/改地址链路经 Windows 装机实跑确认，见 ui-verification/report.md B-4/B-5）
- [x] 8.6 Windows 手工冒烟：安装包双击安装（无管理员弹窗）→ 启动进入应用 → 断网后重试恢复 → 关闭后无残留进程（任务管理器内无 `syntopica-shell`）

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 默认地址可用时正常进入应用 | 人工：ui-verification/report.md B-1（Linux 实跑截图） |
| 按配置切换到局域网地址 | 人工：ui-verification/report.md B-5（待交互环境点击验证） |
| 服务端不要求为壳做改造 | 人工：`git diff --stat backend-go/ front/` 为空（同源 `/api` 既有契约） |
| 服务端停机时显示失败页 | 人工：ui-verification/report.md B-3（含截图） |
| 重试恢复 | 人工：ui-verification/report.md B-4（按钮点击待补） |
| 修改地址后立即生效 | 人工：ui-verification/report.md B-5（按钮点击待补） |
| 冷启动无白闪 | 人工：ui-verification/report.md B-2 |
| Linux 上不出现整体花屏 | 人工：ui-verification/report.md B-6（三页面截图） |
| Windows 上使用默认渲染 | 人工：`grep -n "WEBKIT_DISABLE_DMABUF_RENDERER" desktop/shell/src/main.rs` 仅 Linux 分支命中 |
| 重复双击快捷方式 | 人工：ui-verification/report.md B-7 |
| 关闭窗口后无残留 | 人工：ui-verification/report.md B-8 |
| 普通用户账户安装 | 人工：ui-verification/report.md A-1（Windows 待补） |
| 缺少 WebView2 Runtime | 人工：desktop/shell/src/main.rs `ensure_webview2`（Windows 待补实测） |

- [x] 8.7 Review：change-review-glm 只读审查全量制品与源码，发现建账 `review.md`（R-1~R-11）并逐条处置，验证：`bash scripts/harness/review-trace.sh openspec/changes/add-desktop-shell-remote` 退出码 0
