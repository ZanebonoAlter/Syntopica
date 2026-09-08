# Tasks — fix-doc-impact-misattribution

> 依赖声明：本 change 归档必须晚于 `coordinate-concurrent-changes`（delta spec 叠加关系，见 design Context）。

## 1. 采集侧：edit.map 工具自管文件剔除

- [x] 1.1 `lib/edit-map.ts` 采集路径过滤器：新增黑名单（前缀 `openspec/changes/archive/`、模式 `openspec/changes/<name>/.openspec.yaml`），mergePaths 聚合前剔除工具自管路径；共享文档**不**在此层剔除（design D1），验证：模块内过滤器单元断言（fixture 路径进/出集合清单）
- [x] 1.2 同步入库代码快照到 `docs/research/`（.pi/extensions gitignored），验证：快照文件与 `.pi/extensions/lib/edit-map.ts` 内容一致（diff 为空）

## 2. 消费侧：doc-impact.sh verify 黑名单 + 正则 + 降级

- [x] 2.1 verify 启发式输入黑名单（两轨统一）：工具自管文件 + 仓库级共享文档（`AGENTS.md`、`front/AGENTS.md`、`backend-go/AGENTS.md`、`docs/reference/constraints-index.md`），被剔除路径不触发"疑似遗漏"与"声明 none 但命中"；规则 2/5（checkbox 对账）不受黑名单影响，验证：`bash scripts/doc-impact.smoke.sh` 新增用例通过
- [x] 2.2 configuration 域正则前缀排除：`openspec/` 前缀路径不命中 configuration 域（第二道防线，design D3），真实配置文件命中面不变，验证：smoke 断言 `openspec/changes/configure-x/.openspec.yaml` 不触发、`backend-go/config.yaml` 触发
- [x] 2.3 回退轨降级：heuristic_track=fallback 时启发式命中输出 `[提示] 无法归属: <domain>（全树回退仅提示）` 到 stderr 且不计 fail/不改退出码；ownership 轨命中仍 FAIL；规则 1/2/5 FAIL 不变，验证：smoke 双 fixture（无归属记录 change 全树命中→退出码 0 且有提示；有归属记录命中→退出码非零）
- [x] 2.4 smoke 同 fixture 防漂移（design D2）：对同一组工具自管路径断言"消费侧不触发域命中"，并断言既有 `doc-impact-excuse` 注释读取兼容不报错，验证：`bash scripts/doc-impact.smoke.sh` 退出码 0

## 3. E 段溯源宽限期

- [x] 3.1 `check-standards.sh` E 段增加 `GRACE_DAYS=3` 宽限（GNU date 字典序比较，design D5）：宽限期内 archive change 免检溯源；超期未溯源仍 FAIL；「无 flow 影响」豁免语义不变，验证：`bash scripts/check-standards.smoke.sh` 新增用例（构造宽限期内/超期 fixture）通过

## 4. 文档

<!-- doc-impact: standard -->
- [x] 4.1 更新 `docs/reference/开发执行规范.md` §11.4/§12.2：verify 启发式黑名单与回退降级语义、E 段宽限期（含 GRACE_DAYS 说明）、doc-impact-excuse 废弃状态收口（停止新增，存量兼容），验证：`grep -n "宽限\|无法归属" docs/reference/开发执行规范.md` 命中新增段落
- [x] 4.2 更新 `docs/reference/standard/shared/test-design.md` 或 smoke 相关测试映射（若 change-scope.sh 路径映射涉及 scripts/ 豁免规则则同步），验证：`grep -rn "check-standards.smoke\|doc-impact.smoke" docs/reference/standard/` 引用一致
- [x] 4.3 `docs/research/doc-impact-attribution/explore-findings.md` 追加修复收口注记（四通道对应实现点 + 归档链接占位），验证：文件含"修复收口"节

## 5. 测试

- [x] 5.1 `doc-impact.smoke.sh` 新增四组用例：①归档移动产物（他 change 的 archive/.openspec.yaml）不触发疑似遗漏；②共享文档（AGENTS.md）归属集合内不触发"声明 none 但命中"；③configure-*.openspec.yaml 不命中 configuration；④无归属记录全树回退命中输出提示且退出码 0，验证：`bash scripts/doc-impact.smoke.sh` 退出码 0 且输出含四组新用例名
- [x] 5.2 `check-standards.smoke.sh` 新增两组用例：①宽限期内未溯源不 FAIL；②超宽限期未溯源仍 FAIL，验证：`bash scripts/check-standards.smoke.sh` 退出码 0 且输出含两组新用例名
- [x] 5.3 存量误归因现场回归：对 `openspec/changes/coordinate-concurrent-changes/` 跑 verify，预期从"命中 standard+configuration FAIL"转为"输出无法归属提示且退出码 0"（其无 edit.map 记录，树上 layout.md 为他 change 脏文件），验证：`bash scripts/doc-impact.sh verify openspec/changes/coordinate-concurrent-changes/; echo "exit=$?"` 输出 `exit=0` 且含「全树回退仅提示」

## 6. 验证

### Scenario → 测试映射

| Scenario | 测试文件 |
|---|---|
| 声明了未更新 | scripts/doc-impact.smoke.sh |
| 疑似遗漏 | scripts/doc-impact.smoke.sh |
| 疑似遗漏按归属地图过滤 | scripts/doc-impact.smoke.sh |
| 工具自管文件不触发疑似遗漏 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 共享文档不触发疑似遗漏 | scripts/doc-impact.smoke.sh |
| openspec 元数据不命中 configuration 域 | scripts/doc-impact.smoke.sh |
| 回退全树命中降级为警告 | scripts/doc-impact.smoke.sh |
| 既有 excuse 注释兼容 | scripts/doc-impact.smoke.sh |
| 历史存量豁免 | 人工（check-standards.sh E 段 CUTOFF 代码路径存在，grep -n CUTOFF 命中） |
| 宽限期内未溯源不 FAIL | scripts/check-standards.smoke.sh |
| 超宽限期未溯源仍 FAIL | scripts/check-standards.smoke.sh |
| 无 flow 影响豁免保持 | scripts/check-standards.smoke.sh |
| 编辑路径按绑定 change 聚合 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 归档移动产物不计入归属 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 同文件双 change 触碰标记冲突 | scripts/concurrency-status.smoke.sh |
| 无档会话不计入归属 | .pi/extensions/tests/quality-gate.smoke.cjs |

- [x] 6.1 `bash scripts/doc-impact.smoke.sh` 退出码 0（含全部新增用例）
- [x] 6.2 `bash scripts/check-standards.smoke.sh` 退出码 0（含全部新增用例）
- [x] 6.3 `bash scripts/check-standards.sh --change fix-doc-impact-misattribution` 退出码 0（本 change 自身对账通过）
- [x] 6.4 `bash scripts/concurrency-status.sh --check fix-doc-impact-misattribution` 退出码 0 或 3（无归属其他 active change 的脏文件，冷启动 3 可接受）
