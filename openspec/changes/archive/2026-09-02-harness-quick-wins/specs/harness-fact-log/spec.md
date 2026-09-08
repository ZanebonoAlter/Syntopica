# harness-fact-log Delta

## MODIFIED Requirements

### Requirement: 门禁记账（gate.check）

quality-gate SHALL 对每条实际执行的门禁命令（golangci-lint / go vet / go build / change-scope 判定的 domain go test / pnpm lint）各记一条 `gate.check` 事件，payload 含 cmd、phase、ok、ms；ok=false 时附带截断摘要 diag（≤512B、单行）。

ok=true 事件 SHALL 采样记账（成功运行诊断价值趋零，全量记账造成写放大）：以（session, cmd）为粒度维护连续成功计数——**会话内该命令的首个成功**及失败转绿后的首个成功 MUST 记账（翻转锚点，payload 附翻转标记），其后每 N 次连续成功记 1 条（N 可配置，缺省 5），payload 附采样标记与 N；连续成功段被失败打断后重新起算。ok=false MUST 全量记账，不采样。失败率统计 SHALL 以「失败条数 + 记到的成功条数按锚点 1、采样条 N 加权还原」为分母口径。

同根因短路：golangci-lint 输出含编译失败特征（typechecking error 族）时，quality-gate MAY 跳过本轮必然失败的 go vet 与 domain go test（跳过即未执行）；被跳过的命令 MUST NOT 记 gate.check（未执行零记账，与「纯文档回合零记录」同族语义）。

事件 MUST 绑定 `detectActiveChange` 检测到的活跃 change（无则 null），该检测 MUST 与 constraint-injection 共享同一实现。纯文档/纯对话回合（未触发门禁命令）MUST 零记录。修复回合不单独记账——失败→通过由相邻 gate.check 的 ok 翻转表达。

diag 提取 SHALL 失败特征优先：从命令输出（stdout+stderr 拼接）中优先取首个含失败特征关键词（如 `FAIL`、`error`、`# <pkg>`、`exit` 等关键词表）的行；无任何命中时回退首个非空行。截断规范不变（单行、剥控制字符、≤512 字节）。提取 MUST 确定性（相同输出字节产生相同 diag）。该优先规则仅适用于 gate.check 记账路径；子线程失败白名单的 diag 规范（首个非空错误行）不受影响。

#### Scenario: 门禁命令失败记账

- **WHEN** 某回合 golangci-lint exit 1、go vet exit 0（非短路场景）
- **THEN** 产生两条 gate.check：一条 ok=false 含 512B 内截断 diag，一条 ok=true（按采样规则判定是否落库），两条 change 列均绑当前活跃 change

#### Scenario: 失败全量记账不采样

- **WHEN** 同一（session, cmd）连续 6 次失败
- **THEN** 产生 6 条 ok=false 事件，每条含 diag，无一条被采样跳过

#### Scenario: 转绿翻转锚点必记

- **WHEN** 某（session, cmd）连续失败 3 次后首次成功，随后又连续成功 5 次（N=5）
- **THEN** 转绿首个成功记 1 条（payload 含翻转标记，计 1），其后再记 1 条采样事件（payload 含采样标记与 N=5）；该连续成功段共 6 次运行落库 2 条

#### Scenario: 分母按采样口径还原

- **WHEN** 某（session, cmd）落库 6 条 ok=false、1 条翻转锚点 ok=true、2 条采样 ok=true（N=5）
- **THEN** 统计侧还原总执行次数 = 6 + 1 + 2×5 = 17，失败率 = 6/17（锚点计 1、采样条按 N 加权）

#### Scenario: 同根因短路未执行不记账

- **WHEN** 某回合 golangci-lint exit 1 且输出含 typechecking error，quality-gate 据此跳过 go vet 与 domain go test
- **THEN** 该回合仅产生 1 条 gate.check（golangci-lint，ok=false 含 diag）；go vet / go test 无事件（未执行零记账）

#### Scenario: 未运行不记账

- **WHEN** 回合只改了 docs/ 下 markdown（门禁放行，未执行任何命令）
- **THEN** 该回合不产生任何 gate.check 事件

#### Scenario: diag 失败特征优先（stdout 噪声行不掩盖真实错误）

- **WHEN** golangci-lint exit 1，stdout 首行为 `0 issues.`，stderr 后续行含编译错误（如 `# syntopica-backend/internal/topicgraph/service`）
- **THEN** ok=false 的 gate.check 事件 diag 含编译错误特征行（`# syntopica-backend/...`），而非 `0 issues.`

#### Scenario: go test 记账不丢 FAIL 行

- **WHEN** `go test -short` exit 1，输出首行为父包正常行 `? pkg [no test files]`，后续行含 `FAIL pkg [build failed]`
- **THEN** diag 含 `FAIL` 特征行，可事后审计还原真实失败原因
