## MODIFIED Requirements

### Requirement: init.sh 作为部署唯一入口
系统 SHALL 提供 `deploy/init.sh` 脚本作为用户部署 Syntopica 的唯一入口。脚本 SHALL 在项目根目录执行（以仓库根为工作目录、按仓库相对路径调用 compose），使用 bash（Windows 用户通过 Git Bash）。

#### Scenario: 用户首次部署
- **WHEN** 用户执行 `bash deploy/init.sh`
- **THEN** 脚本启动三阶段流程：核心服务启动 → AI 连接配置 + Firecrawl → 确认并执行

#### Scenario: init.sh 不存在时
- **WHEN** 项目仓库被 clone 但 deploy/init.sh 未执行
- **THEN** 用户可按 README 手动执行 `docker compose -f deploy/compose/docker-compose.yml up -d`，init.sh 非强制

#### Scenario: 重复运行 init.sh
- **WHEN** 用户第二次执行 `bash deploy/init.sh`
- **THEN** .env 合并不覆盖已有值，Provider 通过 upsert 不重复创建
