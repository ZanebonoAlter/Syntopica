# Tasks — tolerate-schema-envelope-extraction-output

<!-- doc-impact: none -->

## 代码

- [x] 1. `extractor_enhanced.go`：`buildExtractionUserPrompt` 末句指令由 `{"tags": [标签列表]}` 改为与真实双数组 schema 一致的表述
- [x] 2. `extractor_enhanced.go`：新增 `unwrapSchemaEnvelope` helper（判定：顶层对象 + `properties` 为对象 + 含 `type`/`required`；幂等；非外壳原样返回）
- [x] 3. `unmarshalMergedExtraction` 入口解外壳；`parseRawTagObjects` 入口解外壳（section 路径与 legacy 回退同享）
- [x] 4. 测试：外壳形态解析用例（merged 双数组 / section 单数组 / properties 内双数组皆空 / 非外壳不误伤）+ prompt 指令与 schema 一致性守护用例（断言 prompt 不含 `{"tags": [标签列表]}`、且提及两个真实键名）
- [x] 5. 门禁：`go build ./...` / `go vet ./...` / `golangci-lint run ./...` / `go test -short ./internal/tagmanagement/...`

## 文档与收尾

- [x] 6. `docs/reference/flow/semantic-board.md`：打标契约节补一句「模型返回 schema 外壳形态由解析器解包」+ prompt 指令须与 schema 一致的纪律
- [ ] 7. `bash scripts/harness/doc-impact.sh verify openspec/changes/tolerate-schema-envelope-extraction-output` + `check-standards.sh`
- [ ] 8. 派 review 子线程审查，意见处置后 `review.md` 建账
- [x] 9. 重启后端 + 真库验证（20 篇试验批，`Force=false`）：**失败 14→0 篇、拿到标签 6→19 篇、含 event 标签 1→18 篇（5%→90%）**；随后批量入队 2502 篇无标签文章（reason=`heuristic_cleanup_retag`，后台跑）
- [ ] 10. 归档 `openspec archive tolerate-schema-envelope-extraction-output` + 补 `docs/reference/flow/semantic-board.md` 变更溯源行

## 重打联动（`retire-heuristic-tag-fallback` 的 tasks 16/17）

- [ ] 11. 本 change 落地后，重打 2488 篇无标签文章（1142 篇本次受影响 + 1345 篇本来无标签），分批观察成功率
- [ ] 12. 重打后核对：board 1980 挂载标签构成、日报文章数恢复、抽样确认拿到真 event 标签
