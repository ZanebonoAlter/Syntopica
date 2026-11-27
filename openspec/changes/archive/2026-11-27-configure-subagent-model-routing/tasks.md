<!-- stage: implement -->

# Tasks

## 1. 配置

- [x] 1.1 建 `.pi/settings.json`：`subagents.defaultModel` + `defaultThinking`
- [x] 1.2 `agentOverrides`：`change-review` / `ui-design` 钉死强模型（glm-5.4）
- [x] 1.3 `agentOverridesByProvider`：minimax-cn / deepseek / zai-coding-cn / openai-codex 四套，各配 general-purpose / explore / plan
- [x] 1.4 五个 agent frontmatter 补 `model:` / `thinking:`
- [x] 1.4b **补 `name:`**（review R-1：缺 name 时 pi-subagents 静默跳过，整套配置零生效）
- [x] 1.5 frontmatter 去重（change-review 原有重复 `thinking` 键）

## 2. 文档

- [x] 2.1 `AGENTS.md`「子线程模型偏好」段改为指向配置 + 四条策略
- [x] 2.2 `开发执行规范.md` §供应商与模型选择 加「配置在哪」表（六层优先级）
- [x] 2.3 保留 `provider/modelId` 全称硬规则（与配置无关）

## 3. 验证

- [x] 3.1 `.pi/settings.json` JSON 合法，四个 provider 层可读
- [x] 3.2 五个 frontmatter 无重复键
- [x] 3.3 `check-standards` 警告清零（顺带补齐 3 个新 spec 的 domain marker）

## 4. review

- [x] 4.1 派 review
- [x] 4.2 处置 review 发现

## 5. 归档

- [ ] 5.1 九项门禁全绿
- [ ] 5.2 `openspec archive configure-subagent-model-routing --yes`
- [ ] 5.3 `openspec validate --specs` 全过

## 6. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| JSON 合法 | `python3 -m json.tool .pi/settings.json` | 通过 |
| 四个 provider 层 | 读 `agentOverridesByProvider` 键 | minimax-cn / deepseek / zai-coding-cn / openai-codex |
| frontmatter 无重复键 | 逐文件解析 | 5/5 OK |
| 文档不再写死模型 | `grep 'MiniMax-M3.1-Flash:max' AGENTS.md` | 0 |
| 文档指向配置 | `grep 'agentOverridesByProvider' 开发执行规范.md` | ≥1 |
| 全称规则保留 | `grep 'provider/modelId' 开发执行规范.md` | ≥1 |

## 7. 文档

<!-- doc-impact: harness=AGENTS.md 与开发执行规范 的模型段改写; dev=配置约定 -->

- `AGENTS.md`（2.1）
- `docs/reference/开发执行规范.md` §供应商与模型选择（2.2）

## 8. 验证

- [x] 8.1 §6 六项全过
- [x] 8.2 实测：派 explore 不指定 model → 走到 `minimax-cn/MiniMax-M3.1-Flash`（返回 429 Token Plan 上限，**证明确实从配置读取**）；源码侧确认 `!frontmatter.name → continue` 是根因

## 9. 验收

- [ ] A 换主力 provider 不需要改任何文件
- [ ] B review 档固定强模型，不随 provider 变
- [ ] C 文档不再写死当前选择
- [ ] D 429 如实报错，不静默换模型
