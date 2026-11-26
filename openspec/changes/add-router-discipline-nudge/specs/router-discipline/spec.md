## ADDED Requirements

### Requirement: 主线程工作外包的行为信号提醒

当主线程出现下列可观测行为时，系统 SHALL 在当下上下文里提醒一次（`steer`，不打断回合）：

| 信号 | 阈值 |
| --- | --- |
| 单次 `read` 的目标行数 | > 200 行（未指定 limit 视作读全文） |
| 同一文件连续 `edit` 次数 | ≥ 3 次 |

提醒 SHALL 说明该派哪类子线程（探索 → `explore`/`plan`；实现与调试 → `general-purpose`）以及主线程与子线程的成本对比。

提醒 MUST NOT 阻断工具执行，MUST NOT 改变任何门禁裁决。同一会话内每类信号 SHALL 至多提醒一次。

#### Scenario: 读大文件触发提醒

- **WHEN** 主线程调用 `read` 且目标超过 200 行（含未指定 `limit`）
- **THEN** 系统 SHALL 投递一条提醒，建议派 explore 子线程并把结论 `pin_finding` 落盘

#### Scenario: 连续 edit 触发提醒

- **WHEN** 主线程对同一文件连续第 3 次 `edit`
- **THEN** 系统 SHALL 投递一条提醒，指出这是调试循环、该派 general-purpose

#### Scenario: 换文件重置计数

- **WHEN** 主线程编辑了另一个文件
- **THEN** 前一文件的连续 edit 计数 SHALL 重置

#### Scenario: 不阻断

- **WHEN** 任一信号触发
- **THEN** 对应的 `read` / `edit` SHALL 正常执行，MUST NOT 因提醒被拒绝

### Requirement: 豁免必须带理由并留痕

主线程可通过 `bash scripts/harness/router-discipline.sh --excuse "<理由>"` 声明本会话豁免。

- 理由为空时 MUST 拒绝豁免（exit 2）
- 豁免生效后，本会话 MUST NOT 再投递提醒
- 豁免 SHALL 记一条 `policy=router-discipline`、`action=bypass`、`reasonCode=router-excused`、`detail=<理由>` 到 harness 事实库
- `--status` SHALL 显示当前豁免状态与理由；`--clear` SHALL 清除豁免

#### Scenario: 带理由豁免

- **WHEN** 执行 `--excuse "改一个 typo，单行改动，不值得派子线程"`
- **THEN** 本会话后续 SHALL 静音提醒，且事实库 SHALL 有带该理由的 bypass 记录

#### Scenario: 空理由不豁免

- **WHEN** 执行 `--excuse ""`
- **THEN** 命令 SHALL 报错退出（exit 2），提醒不静音

#### Scenario: 豁免不静默

- **WHEN** 豁免已生效
- **THEN** `--status` SHALL 显示豁免时刻与理由文本，供事后抽查是否被滥用

### Requirement: 分工判据写入执行规范

`开发执行规范.md` SHALL 明确跨界表：探索 / 实现 / 验证外包给子线程，判断 / 验收留主线程。

#### Scenario: 判据可查

- **WHEN** 开发者在主线程动手前查规范
- **THEN** SHALL 能看到五类活的归属表与豁免命令
