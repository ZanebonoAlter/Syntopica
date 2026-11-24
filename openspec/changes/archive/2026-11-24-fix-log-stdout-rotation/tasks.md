## 1. logging 包改造

- [x] 1.1 `logging` 包增加 `FileWriter() io.Writer` 导出（文件日志未启用时返回 `nil`），并让 `Init` 在 `fileCfg.Enabled && Path != ""` 时**只**装文件 handler、不再无条件扇出 stdout；`LOG_CONSOLE=true` 时恢复 console handler。验证：`cd backend-go && go build ./...` 通过且 `go test ./internal/platform/logging` PASS。
- [x] 1.2 在 `logging` 包单测中覆盖三条路径：文件启用时不写 stdout、`LOG_CONSOLE=true` 时同时写 stdout、文件未启用时行为与改动前一致（纯 stdout）。验证：`cd backend-go && go test ./internal/platform/logging -run TestInit -v` 全绿。

## 2. 接入 Gin 访问日志

- [x] 2.1 `cmd/server/main.go` 在 `logging.Init` 之后、`r := gin.Default()` 之前设置 `gin.DefaultWriter` 与 `gin.DefaultErrorWriter` 为 `logging.FileWriter()`（非 nil 时）；并在启动横幅可见处打印一行当前日志文件路径。验证：`cd backend-go && go build ./...` 通过；重启后 `grep -c '^\[GIN\]' logs/app.log` 大于 0。
- [x] 2.2 确认 `gin.Default()` 的 Logger/Recovery 中间件确实使用注入的 writer（而非捕获默认值）：重启后连续请求 `/api/poll` 三次，验证 `logs/app.log` 新增 3 条 `[GIN]` 记录且 `nohup.out` 无新增。

## 3. 配置与启动脚本

- [x] 3.1 `backend-go/configs/config.yaml` 的 `log.level` 由 `debug` 改为 `info`；`docs/reference/configuration.md` 记录「改级别 = 改该值后重启」。验证：`grep -n 'level: "info"' backend-go/configs/config.yaml` 命中，且后端启动日志不含 `TagMatch: start` 的 DEBUG 级冗余（INFO 计数下降）。
- [x] 3.2 `scripts/dev/start-dev.sh` 头部注释与日志提示改为 `tail -f backend-go/logs/app.log`（保留 nohup.out 仅作启动兜底的说明）。验证：`grep -n 'logs/app.log' scripts/dev/start-dev.sh` 命中且不再宣称 nohup.out 是主日志。

## 4. 测试

- [x] 4.1 新增 `backend-go/internal/platform/logging/logging_stdout_test.go`（或扩展现有测试文件）覆盖：`Init(level, FileConfig{Enabled:true,...})` 后 stdout 不可见、`LOG_CONSOLE=true` 可见、`FileWriter()` 非 nil。验证：`cd backend-go && go test ./internal/platform/logging -v` PASS。
- [x] 4.2 验证 `SetWriters`/`ResetWriters` 既有测试未回归。验证：`cd backend-go && go test ./internal/platform/logging` 全绿。

## 5. 文档
<!-- doc-impact: configuration -->
<!-- doc-impact-excuse: database=工作树里另有其它 change 的 backend-go/internal/models/ 改动（非本 change）；本 change 只动 logging 包与 server 启动装配，未改 schema -->

- [x] 5.1 `docs/reference/configuration.md` 增加「日志」小节：文件路径、轮转参数（50MB×30×30 天×gzip）、`LOG_CONSOLE` 开关、级别调整方式，并说明 `nohup.out` 只承载启动横幅。验证：`grep -n 'logs/app.log' docs/reference/configuration.md` 命中。
- [x] 5.2 `docs/reference/development.md` 的日志查看指引更新为 `tail -f backend-go/logs/app.log`；如提到 `nohup.out` 则标注为启动兜底。验证：`grep -rn 'nohup.out' docs/reference/development.md` 无「查看运行日志」类表述。
- [x] 5.3 归档前对本 change 无业务域声明做一次确认（日志管道不改变任何 flow 行为）：`bash scripts/harness/doc-impact.sh verify --change fix-log-stdout-rotation`，期望报告「无声明域」或全部已覆盖，无缺口。

## 6. 验证

- [x] 6.1 静态检查：`cd backend-go && golangci-lint run ./... && go vet ./...`，期望 0 issues。
- [x] 6.2 针对性测试：`cd backend-go && go test ./internal/platform/logging ./cmd/...`，期望 PASS（`./cmd/...` 无测试时显示 no test files 也算通过）。
- [x] 6.3 构建：`cd backend-go && go build ./...`，期望退出码 0。
- [x] 6.4 运行验证：`bash scripts/dev/start-dev.sh --restart back` 后执行 `ls -l backend-go/nohup.out backend-go/logs/app.log`，期望 nohup.out 小于 50KB 且 app.log 持续增长；`curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:5100/health` 返回 200。
- [x] 6.5 回归验证：请求 `curl -s http://127.0.0.1:5100/api/poll >/dev/null` 三次后 `tail -3 backend-go/logs/app.log` 出现三条 `[GIN]` 记录，`grep -c 'GIN' backend-go/nohup.out` 不增加。
- [x] 6.6 一致性校验：`grep -rn 'nohup.out' scripts/dev/start-dev.sh docs/reference/implementation*.md docs/reference/development.md docs/reference/configuration.md`，期望所有命中均为「启动兜底」表述，无「主日志」表述。
