<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档结构重构**：

- 改动范围只有 `openspec/specs/` 下的文件边界（23 个标签 capability 合并成 3 个族级），不产生任何界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- 合并后的 spec 内容逐字保持原样（review 已全量比对 133/133 一致），用户可见行为零变化
- 唯一的外部可见影响是 `domain-history.sh` 反查结果与 `docs/reference/` 里的旧 capability 路径引用（tasks 7.4 处理）
