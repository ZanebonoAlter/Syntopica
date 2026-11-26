<!-- stage: implement -->

# Tasks

## 1. 规范先落地

- [x] 1.1 `prototype-content.md` 新增「原型类别与可操作性下限」：两类原型（functional / exploratory）定义与声明方式
- [x] 1.2 设计下限四条（热区 24×24 / 焦点 / 悬停 / 键盘）写明**人核、不做机器检查**及原因
- [x] 1.3 WC 危险操作确认：唯一机器检查项，双通道判据 + 确认机制三种
- [x] 1.4 撤回原 EC-1/EC-2/EC-3（评审实测为形式主义：一处声明全过、`outline:none` 过关）

## 2. 检查脚本

- [x] 2.1 `ui-prototype-check.sh` 门户链：legacy → major → exploratory → 有原型目录 → 根元素 skip
- [x] 2.2 扫描范围限定顶层 `*.html` + `skeleton/**`（不递归实验子目录）
- [x] 2.3 破坏性识别三通道：文字 / `data-destructive` / `input value`
- [x] 2.4 确认机制三种：同标签 `data-confirm`/`confirm(` / `<dialog>` / 邻近 3 行「确认」
- [x] 2.5 豁免只认 `<html>`/`<body>` 根元素，理由非空才生效（R-8）
- [x] 2.6 输出 `file:line` + 元素片段（R-9）
- [x] 2.7 **所有文本判定改用 here-string，不走 `pipefail` 下的管道**（R-1：曾让同一输入跑出 5 种裁决）

## 3. 挂载

- [x] 3.1 `spec-gate.ts` 检查⑨（spawn，fail-open）
- [x] 3.2 `archive-readiness.sh` 第⑨项，八项→九项
- [x] 3.3 `archive-readiness.smoke.sh` 断言更新（16 case 全过）
- [x] 3.4 `开发执行规范.md` EC 节重写为「只查危险操作确认」

## 4. 测试

- [x] 4.1 `ui-prototype-check.smoke.sh` 21 断言全过
- [x] 4.2 **R-1 回归**：100KB 输入连跑 5 次裁决一致（smoke #20/#21）
- [x] 4.3 R-1 场景手工复验：20 次连跑全 `0`（曾 30 次 5 种裁决）
- [x] 4.4 现存 three.js 原型（legacy）放行

## 5. review

- [x] 5.1 派 review
- [x] 5.2 处置 review 发现

## 6. 归档

- [ ] 6.1 九项门禁全绿
- [ ] 6.2 `openspec archive add-ui-prototype-mechanics-gate --yes`
- [ ] 6.3 `openspec validate --specs` 全过

## 7. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 合规原型 | smoke：含尺寸声明 + focus + hover + 确认 | exit 0 |
| 缺尺寸 | smoke：button 无对应规则 | exit 1，列出元素 |
| 缺 focus | smoke：CSS 无 :focus | exit 1 |
| 缺 hover | smoke：CSS 无 :hover | exit 1 |
| 危险无确认 | smoke：`<button>删除</button>` 无确认链路 | exit 1 |
| 危险有确认 | smoke：同上 + `data-confirm` | exit 0 |
| 跳过标记 | smoke：`data-ui-mechanics-skip="理由"` | exit 0 + 回显理由 |
| 空理由 | smoke：`data-ui-mechanics-skip=""` | 照常检查（exit 1） |
| 非 major | smoke：`ui-impact: minor` | exit 0 |
| 无原型目录 | smoke：major 但无 `ui-prototype/` | exit 0 |
| 不扫实验文件 | smoke：目录里放 png/py，检查不报它们 | exit 0 |

## 8. 文档

<!-- doc-impact: dev=harness 门禁脚本与新扩展; standard=prototype-content.md 新增可操作性下限节 -->

- `docs/reference/standard/frontend/prototype-content.md`（1.1–1.3）
- `docs/reference/开发执行规范.md` §11 门禁清单（3.4）
- `docs/reference/harness/pi-extensions.md` 门禁分层表加检查⑨（可选）

## 9. 验证

- [x] 9.1 smoke 全绿（§7 逐项）
- [x] 9.2 现存原型实跑不误报（4.2）
- [x] 9.3 `check-standards` 通过数不降

## 10. 验收

- [x] A 四项检查各自能红能绿，理由可读
- [x] B 挂载点真能拦（spec-gate 阻断实测）
- [x] C 24px 取值有权威出处，不再只活在台账
- [x] D 存量原型不被实验产物连累
