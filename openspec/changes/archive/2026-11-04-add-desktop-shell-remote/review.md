# Review 记录 — add-desktop-shell-remote

> 审查人：change-review-glm（zai glm-5.3 high）只读审查，2026-11-03。
> 范围：全部 6 制品 + shell 源码 + 三处文档 + 红线四条；报告全文见当回合会话记录，以下为建账与处置。

### R-1 2026-11-03【High】首帧背景色（D7/白闪）零实现，task 1.2 与报告 B-2 失实
- **状态**：已修复
（visible(false) 建窗 → theme() 探测系统主题 → 深色 set_background_color(#080c12) → show()；令牌取自 main.css --color-bg-base。慢服务端实测首帧主色 #faf7f2，证据 ui-verification/linux-shell-firstframe-theme.png；tasks 1.2 验证命令改为可判定检查）

### R-2 2026-11-03【High】B-4「重试恢复」验收证据不成立：引用不存在的单测，「待补」与全勾自相矛盾
- **状态**：已修复
（report B-4/B-5 重写：删除虚构单测引证；Windows 装机实跑「失败页→改地址→进应用」覆盖 probe_server+导航恢复链（用户确认 2026-11-03）；重试按钮点击终验列入 Windows 重装轮验收步骤；「待补」字样与 tasks 映射表对齐清除）

### R-3 2026-11-03【High】配置读写测试全缺位，tasks 2.1/2.4 验证声明无落点
- **状态**：已修复
（config.rs 抽出 read_config/write_config 路径参数化；新增 save→load 往返/覆写/无残留 tmp、损坏文件 Err 且不覆写、只读目录可见错误（unix）三组真实用例；tasks 2.1/2.4 措辞对齐实际用例名）

### R-4 2026-11-03【Medium】ensure_webview2 只查 HKLM WOW6432Node，per-user（HKCU）安装会被误判缺失
- **状态**：已修复
（补查 HKCU 同 GUID，任一命中即视为已装；随 Windows 重装轮生效）

### R-5 2026-11-03【Medium】初始探测成功后的会话内导航失败无壳内兜底
- **状态**：已采纳
（spec Requirement 显式限缩到「探测时点（启动/重试/改地址）」+ 范围说明注明会话内失效为已知限制；README 已知限制同步两条；会话内导航失败监听留给后续 sidecar change，不在本 change 强行扩面）

### R-6 2026-11-03【Medium】complex 档缺 Review 任务行，check-tasks-tail 实测 exit 1
- **状态**：已修复
（tasks 新增 8.7 Review 行并勾选；本文件即建账）

### R-7 2026-11-03【Low】probe 单次 read(512) 解析状态行，无「读到首行完整」循环
- **状态**：已修复
（nav.rs 改循环读至 \r\n 或 1024B 上限，分段吐字节/超长首行不再误分类）

### R-8 2026-11-03【Low】current_target 命令注册但从未被调用
- **状态**：已采纳
（已删除，留给后续 sidecar change 按需引入）

### R-9 2026-11-03【Low】csp: null 且本地错误页无 CSP meta
- **状态**：已修复
（tauri.conf.json 设最小 CSP：default-src 'none' + script-src 'self' 'unsafe-inline' + connect-src ipc: http://ipc.localhost 等；因 Tauri 设 CSP 后会向 script-src 注入自家脚本 hash（hash 存在时浏览器忽略 unsafe-inline），error.html 内联脚本外置为本地 error.js 由 'self' 覆盖；A/B 实测错误页在 CSP 下渲染正常，证据 linux-shell-error-csp.png；invoke 点击链终验列入 Windows 重装轮）

### R-10 2026-11-03【Low】两处弱断言测试：尾斜杠/路径保留未断言、超长 URL 用例恒真
- **状态**：已修复
（ipv6 用例补精确断言（尾斜杠补齐/带路径不追加/已有保持）；超长用例改封闭结论：≤8KB 接受、>8KB 拒绝（新增 MAX_URL_LEN=8KB 壳层防御））

### R-11 2026-11-03【Low】启动探测在 setup 内同步阻塞至多 5s；首窗未建时单实例聚焦静默落空
- **状态**：已采纳
（README 已知限制记录两条边界：黑洞地址窗口迟到 5s、期间二次启动不聚焦；异步探测改造收益不抵复杂度，留待真实卡顿反馈再议）

---

## 审毕无发现方面（摘自审查报告）

红线四条（front/backend-go 零改动、远程模式契约、协议与平台配置、地址优先级逐级回落）逐项核过均未违反；capabilities 最小化（仅 core:default@main，无 remote 授权，远端页面拿不到 IPC）；error.html 注入面（query 全走 encode + textContent）；config 平台目录与原子写语义；文档三方对账无漂移；scenario-trace 13 项机器对账通过；A-4 标 N/A 理由链成立；单实例/关闭即退出；工程杂项（Cargo.lock 已提交、icons 存在、target 已 ignore）。
