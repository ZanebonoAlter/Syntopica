# FB6：已批准总览实施计划

用户明确授权zai-coding-cn/glm-5.3 max按原型修改总览。ui-design.md的approved仅本阶段总览范围，矩阵等仍未批准；本计划不缩减原change任何剩余验收责任。

## 范围与顺序

1. 以当前未提交源码为基线，保留真实API/路由/治理动作；参考已批准总览的维护源styles.css/prototype.js与最终1440/2560图，不复制离线数据、原型导航或intent占位。
2. 先继承/补充BoardRadarView定向用例：报刊层级、完整横比/常温索引展开、升温/截止、读报/泳道去向、无泳道有日报。原有排序/阈值/边界断言不能删除以洗绿。
3. 实现总览簇。必要拆同域展示组件/composable保持文件规模，必要API边界转换仅Radar分支/normalizer；后端字段与统计不改。原型的矩阵/日报等部分不移进产品。
4. 若规范化客户端形态，独立验证真实snake_case响应→camelCase/字符串ID，再让组件消费；不能只改mock形态但漏接真实API。
5. 只跑影响单测（maxWorkers=2、参数无额外`--`）与受影响文件eslint；父线程收尾统一typecheck/构建、静态实页和独立Review。子线程不自行部署、重启或跑全量。

## Scenario → 测试文件

以下继承 `specs/workshop-radar/spec.md`，数据语义不改，不重开后台聚合设计。

| Scenario | 测试文件 |
| --- | --- |
| 打开工坊首屏 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 从雷达进入版块 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 热点行带理由与密度 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 常温版块折叠 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 升温判定 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 无升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 一次请求渲染雷达 | front/app/features/tags/components/console/BoardRadarView.test.ts（必要API边界测试另加，不用N路查询） |
| 无数据版块 | front/app/features/tags/components/console/BoardRadarView.test.ts |

补充完整故事：总览→常温展开→有日报无泳道的板块→指定日报；总览泳道小矩阵→既有?lane=控制台；全局治理菜单→保留已有dialog/取消。组件测路由/事件/数据，父线程静态实页用agent-browser验证实际打开状态、两主题与宽窄布局，不把DOM存在当完成。

## 继承与白盒

test-cases.md追加本阶段继承表：旧测试保留/因已批准文案和层级变化调整的具体断言/新增断言。白盒覆盖分档边界50/15、前三日0时环比、新数据/无数据/旧数据失败、不同锚日、0泳道但有日报、长标签/正常返回；新增helper在最便宜层测试。不得顺手处理原9.3/9.4/9.5或退役8.4，关系等旧能力不删。

## 并发与收口

根当前另有约45个前端脏文件，目标RadarVue/test本身未跟踪、semanticBoards.ts已有修改。它们是现有基线，不得restore/覆盖；API只精确修改Radar区。基线副本供父线程核对本阶段增量。部署可能夹带他人改动，父线程先判定发布范围，不让子线程全仓build/重启。

此阶段只完成总览源码/定向用例后交父线程验证，不标整个change完成、不改根UI账本或关闭全部review。矩阵新要求见ui-design-matrix-draft.md和任务13，保持设计阶段。
