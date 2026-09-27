## MODIFIED Requirements

### Requirement: 前端 dev server 绑定全部接口

前端 dev server SHALL 显式绑定 `0.0.0.0`，使本机任意网卡地址（含局域网、虚拟网段与容器网桥）与 IPv4 环回均可访问：默认 `localhost` 绑定在部分平台（Windows）解析为 `::1` 仅监听 IPv6 环回，IPv4 loopback 与跨主机访问均无法到达。

#### Scenario: WSL 可达前端 dev server
- **WHEN** dev server 启动后从本机 `http://127.0.0.1:3000/` 与局域网其他主机 `http://<开发机IP>:3000/` 访问
- **THEN** 两处请求均成功（IPv4 全接口监听，非 v6-only）

### Requirement: 开发网络访问口径文档化

开发文档 SHALL 记录：后端默认 5100（浏览器/工具直连 + CORS 白名单）、localhost 探测卫生约定（当开发主机存在系统代理时，探测 `localhost` / `127.0.0.1` / `::1` MUST 绕过代理——设 `no_proxy` 或用 `curl --noproxy '*'`，防止代理劫持本地探测产生假象）。该约定 SHALL 表述为平台中立（任何配置了系统代理的开发主机均适用），MUST NOT 绑定到某个特定的跨系统运行环境。

#### Scenario: 文档口径一致
- **WHEN** 开发者查阅 AGENTS.md / docs/reference/development.md / configuration.md / deployment.md
- **THEN** dev 模式的口径为「后端默认 5100、前端 dev server 绑 0.0.0.0、绝对 base 直连后端 origin」，无残留「dev 走同源代理 / 5000 端口」的过时指引（同源反代是独立的**部署**形态，见 `deploy/same-origin/`，不属 dev 口径）

#### Scenario: 探测卫生约定平台中立
- **WHEN** 对 `docs/reference/` 检索 localhost 探测卫生约定所在段落
- **THEN** 该约定以「存在系统代理时」为条件表述，不出现仅对某一跨系统运行环境生效的措辞
