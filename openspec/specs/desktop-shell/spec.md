<!-- domain: ui -->

# desktop-shell Specification

## Purpose

定义桌面壳（desktop shell）在远程模式下的可观察行为：窗口装载现有 SPA、连接失败时的兜底与恢复、各平台渲染适配、单实例与分发约束。它是后续「壳 + 本地 sidecar」形态的公共底座，本 capability 只覆盖远程装载这一层。

## Requirements

### Requirement: 窗口按配置地址装载现有 SPA

壳 SHALL 在启动时按配置的目标地址装载 SPA，地址来源按优先级为：启动参数 / 环境变量 / 用户本地配置 / 默认 `http://127.0.0.1:5100/`。装载的页面 MUST 与浏览器直接访问同一地址时功能一致；壳 MUST NOT 要求服务端为壳做任何接口或鉴权改动（同源 `/api` 契约保持不变）。

#### Scenario: 默认地址可用时正常进入应用

- **WHEN** 壳启动且默认地址的服务端可达
- **THEN** 窗口完成导航并呈现 SPA 的阅读页，其后续请求全部发往同一 origin 下的 `/api`

#### Scenario: 按配置切换到局域网地址

- **WHEN** 用户将目标地址配置为 `http://<lan-host>:5100/` 并重启壳
- **THEN** 壳装载该地址，行为与默认地址一致，且不因跨主机而产生额外的跨域预检失败

#### Scenario: 服务端不要求为壳做改造

- **WHEN** 服务端仍以既有静态托管方式提供页面与 `/api`
- **THEN** 壳与浏览器访问得到相同的功能与数据，服务端无需新增鉴权或 CORS 条目

### Requirement: 远端不可达时提供可恢复的失败页

在壳的**探测时点**（启动、重试、修改地址）目标地址不可达、连接超时或返回非成功状态时，壳 SHALL 显示连接失败页，页面 MUST 说明失败原因类别，并提供「重试」与「修改地址」两个动作。失败页 MUST NOT 依赖远端资源（字体、图标、样式内联，脚本为本地打包的同目录文件，零外链）。重试成功后 MUST 进入正常应用，无需重启壳。

> 范围说明：探测通过后**会话期间**服务端再失效（加载中断、TLS 中途损坏等）时，WebView 将呈现其原生错误页（无壳内重试入口），重启壳即可回到失败页流程——这是远程模式当前已知限制（见 `desktop/README.md`），不往本 Requirement 覆盖范围内延伸；会话内导航失败监听留给后续 sidecar change。

#### Scenario: 服务端停机时显示失败页

- **WHEN** 壳启动且目标地址拒绝连接
- **THEN** 窗口显示连接失败页，包含原因说明、重试与修改地址入口，且页面在无网络时也能完整渲染

#### Scenario: 重试恢复

- **WHEN** 失败页显示期间服务端恢复可用，用户点击「重试」
- **THEN** 壳重新导航并在成功后呈现 SPA，无需关闭窗口

#### Scenario: 修改地址后立即生效

- **WHEN** 用户在失败页修改目标地址为另一可用地址并确认
- **THEN** 壳装载新地址并进入应用，且该地址被持久化为后续启动的默认值

### Requirement: 首帧不出现白闪且与主题一致

窗口创建到远端 HTML 到达之间，壳 SHALL 以当前主题的背景色填充窗口，MUST NOT 出现默认白色底闪烁。远端 HTML 到达后，应用内首屏加载表现 SHALL 由既有 SPA 首屏模板承接，壳不重复实现应用内加载态。

#### Scenario: 冷启动无白闪

- **WHEN** 在深色主题下冷启动壳
- **THEN** 首帧为深色背景，不出现白色闪屏，随后过渡到 SPA 首屏加载态

### Requirement: 平台渲染适配

壳 SHALL 在 Linux（WebKitGTK）上禁用 DMABUF 渲染器后再创建窗口；在 Windows（WebView2）与 macOS 上 MUST 使用平台默认渲染配置。该适配 MUST 对用户不可见、无需用户手动设置。

#### Scenario: Linux 上不出现整体花屏

- **WHEN** 在 Linux/arm64 桌面启动壳并打开阅读页、日报页与矩阵页
- **THEN** 三个页面均正常渲染，不出现整窗条纹或内容错位

#### Scenario: Windows 上使用默认渲染

- **WHEN** 在 Windows 启动壳
- **THEN** 不注入任何渲染降级开关，页面由 WebView2 默认配置渲染

### Requirement: 单实例运行

同一用户下壳 MUST 只运行一个实例；重复启动时 MUST 聚焦已存在窗口而不是创建第二个窗口或第二份数据写入路径。

#### Scenario: 重复双击快捷方式

- **WHEN** 壳已在运行时再次启动
- **THEN** 已存在窗口被聚焦，系统中不出现第二个壳窗口

### Requirement: 关闭即退出且不留残留进程

关闭最后一个窗口时壳 SHALL 结束自身进程树；MUST NOT 留下后台进程继续持有端口或占用资源。

#### Scenario: 关闭窗口后无残留

- **WHEN** 用户关闭壳窗口
- **THEN** 壳进程在数秒内退出，系统中不存在该壳的残留进程

### Requirement: Windows 分发约束

Windows 安装包 MUST 为 per-user 安装（不要求管理员权限），且 MUST 在缺少 WebView2 Runtime 时给出可执行的安装引导而不是静默失败。安装包 MUST NOT 内嵌固定版本 WebView2。

#### Scenario: 普通用户账户安装

- **WHEN** 非管理员账户双击安装包并完成安装
- **THEN** 安装成功、开始菜单出现入口，启动后进入应用

#### Scenario: 缺少 WebView2 Runtime

- **WHEN** 目标机器未安装 WebView2 Runtime
- **THEN** 壳启动时提示缺失并提供 Evergreen 安装引导，而不是显示空白窗口或崩溃
