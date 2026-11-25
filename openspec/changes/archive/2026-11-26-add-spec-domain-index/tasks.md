# Tasks — add-spec-domain-index

## 1. 归类定稿

- [x] 1.1 子线程按 Purpose + Requirement 判 177 个 capability 的域归属，产出 `spec-domains-draft.md`（v2，带 Purpose）
- [x] 1.2 复核 `draft` 的 `l` / `?` 条目并从抽查中修正明显误判，定稿域归属
- [x] 1.3 抽查 h 档 ≥15 条：Purpose 与 domain 自洽率 ≥90% 才进入批量写入

## 2. 批量写入 marker

- [x] 2.1 按定稿表给 177 个 `openspec/specs/*/spec.md` 头部写 `<!-- domain: ... -->`；幂等：已有 marker 的跳过，不覆盖人工修订
- [x] 2.2 校验脚本：177 个全部有 marker、域名 100% 落在 15 词表内、多域 ≤3 个

## 3. 反查脚本

- [x] 3.1 `scripts/harness/domain-history.sh`：五段输出（spec 清单 / 时间线 / flow 红线 / REMOVED / MODIFIED），退出码恒 0
- [x] 3.2 `scripts/harness/domain-history.smoke.sh`：fixture 10 组 case 全过（`DOMAIN_HISTORY_ROOT` 覆盖根）

## 4. 规范与门禁登记

- [x] 4.1 `docs/reference/standard/shared/doc-authoring.md` 注册点速查表加 `<!-- domain: -->` 一行（含词表与位置约定）
- [x] 4.2 `docs/reference/harness/pi-extensions.md` 脚本清单加 `domain-history.sh`
- [x] 4.3 `scripts/harness/check-standards.sh` 加 warn 段：归档时若本 change 的 delta 引入新 capability 而该 capability 的 main spec 无 domain marker → 提示（**不 block**，观察期）

## 5. Review

- [x] 5.1 派子线程聚焦 review（制品 + diff），意见按 High/Medium/Low 处置后在 `review.md` 建账

## 6. 测试

- [x] 6.1 `bash scripts/harness/domain-history.smoke.sh` → 全部通过
- [x] 6.2 `bash scripts/harness/check-standards.smoke.sh` → 全部通过（改了 check-standards 时）
- [x] 6.3 `bash -n scripts/harness/domain-history.sh` → 无语法错误

## 7. 文档

<!-- doc-impact: standard -->

- [x] 7.1 `docs/reference/standard/shared/doc-authoring.md`（注册点速查）
- [x] 7.2 `docs/reference/harness/pi-extensions.md`（脚本清单）
- [x] 7.3 `docs/research/process-arch-rework/issues.md`（ISS-13 状态回填为已落地）
- [x] 7.4 归档合并后给本 change 的 `spec-domain-index` main spec 补 `<!-- domain: dev -->`（R-7：归档前不存在该文件，J 段 WARN 属预期）

## 8. 验证

- [x] 8.1 `bash scripts/harness/domain-history.sh semantic-board` → 第 1 段列出 ≥2 条 spec，第 4/5 段有条目，退出码 0
- [x] 8.2 `grep -l '<!-- domain:' openspec/specs/*/spec.md | wc -l` → 177
- [x] 8.3 `bash scripts/harness/domain-history.sh --list` → 15 个域名
- [x] 8.4 `openspec validate add-spec-domain-index` → valid
- [x] 8.5 抽 3 个域跑反查，确认历史时间线与 `git log` 事实一致（人工比对）
- [x] 8.6 **草表 ↔ 落盘 marker 全量对账**（R-2，长期保留命令）：`diff <(grep -E '^\| [a-z0-9-]+ \| [0-9]+ \| ' openspec/changes/add-spec-domain-index/spec-domains-draft.md | awk -F' \| ' '{sub(/^\| /,"",$1); print $1"\t"$3}' | tr -d ' ' | sort) <(for f in openspec/specs/*/spec.md; do printf '%s\t%s\n' "$(basename "$(dirname "$f")")" "$(sed -n '1,8{/<!--[[:space:]]*domain:/{s/.*domain:[[:space:]]*//;s/-->.*//;s/[[:space:]]*$//;p;q}}' "$f")"; done | tr -d ' ' | sort)` → 无输出（177 条完全一致）
- [x] 8.7 `bash scripts/harness/domain-history.smoke.sh` → 19 项；`bash scripts/harness/check-standards.smoke.sh` → 19 项
