# Ollama 空 auxiliary_labels 降级容错

## What to build

当前 `parseAuxiliaryLabels`（extractor_enhanced.go）对 event/person 标签要求 auxiliary_labels 数量 3-5 个，为空则报错导致整个 tag 提取失败并触发 retry。Ollama 模型对 JSON Schema 遵守度不足，经常返回空的 auxiliary_labels，导致 retry 3 次后整体失败。

改为：event/person 标签 auxiliary_labels 为空时，降级保留标签（不带锚点），不阻断提取流程，打 warn 日志记录降级事件。有值时仍走原有校验逻辑（3-5 个、不能太泛等），不降低正常输出的质量标准。

## Acceptance criteria

- [ ] event/person 标签 auxiliary_labels 为空或 null 时，返回空 slice 而非报错
- [ ] 打 warn 日志记录降级事件（包含标签 label、category、provider 信息）
- [ ] 有值时仍走 3-5 个数量校验、泛词过滤、description 非空等现有规则
- [ ] 重试逻辑不受影响：有值但不足 3 个时仍触发 retry
- [ ] keyword 分支行为不变（本来就不要求 auxiliary_labels）
