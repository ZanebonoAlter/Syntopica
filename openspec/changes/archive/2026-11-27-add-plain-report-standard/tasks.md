# Tasks

## 1. 规范落地

- [x] 1.1 `docs/reference/standard/shared/doc-authoring.md` 新增「汇报叙事：先给人话（EIAL5）」节
- [x] 1.2 四种最常翻车场合的正反例（change 完工 / review 收口 / marker 理由 / 派发前说明）
- [x] 1.3 自检判据：把第一段读给不熟本项目的人听，能否复述出改动范围
- [x] 1.4 明确「不做机械校验」及其理由（只能判字样在在的检查会催生应付式写法，同 ISS-03 教训）

## 2. 注册点

- [x] 2.1 `AGENTS.md` 汇报纪律新增一条，指向该节（不复制正文，避免两处漂移）
- [x] 2.2 原有的「部署后影响 + 需要的操作」条保留不变（那是内容要求，EIAL5 是措辞要求）

## 3. review

- [x] 3.1 派 `change-review` 审（自指检验：用它自己的规范写成人话版）
- [x] 3.2 处置 7 条（1 High：必经路径没接上，已接通三处）

## 4. 归档

- [x] 4.1 九项门禁全绿
- [ ] 4.2 执行归档命令
- [ ] 4.3 `openspec validate --specs` 全过

## 5. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 规范可查 | `grep '汇报叙事' docs/reference/standard/shared/doc-authoring.md` | 命中 |
| AGENTS 指向 | `grep '汇报先给人话' AGENTS.md` | 命中且含链接 |
| 无机械校验 | `grep -i 'lint\|门禁' doc-authoring.md` 的该节 | 只出现「不做机检」的说明，无检查脚本引用 |
| 正反例可照抄 | 逐组读：反例是否术语堆叠、正例是否说清改动范围 | 四组都成立，无凑数 |

## 6. 文档

<!-- doc-impact: harness=新增汇报叙事规范并接入 AGENTS.md -->

- `docs/reference/standard/shared/doc-authoring.md`（1.x）
- `AGENTS.md`（2.1）

## 7. 验证

- [x] 7.1 §5 四项全过
- [x] 7.2 台账 ISS-11 状态更新为已落地，Q1/Q2 改「落定」

## 8. 验收

- [x] A 规范里有可照抄的正反例（四组，同一件事的正反对照）
- [x] B 明确不做机检并说清两个洞（黑名单不全 / 挡不住空话首段）
- [x] C 位置在必经路径上：doc-authoring（写文档必读）+ catalog 路由的「注册点速查」加了指路 + AGENTS.md 两处引用
