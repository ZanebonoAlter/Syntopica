#!/usr/bin/env bash
# stage-agent.sh 冒烟：六阶段路由 + 边界兜底 + marker 解析边界。
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.." || exit 1
S="scripts/harness/stage-agent.sh"
PROBE="openspec/changes/_smoke_stage_agent"
pass=0; fail=0
ok()  { pass=$((pass+1)); printf '  ok  %s\n' "$1"; }
bad() { fail=$((fail+1)); printf '  FAIL %s\n' "$1"; }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1（期望 $3，实得 $2）"; fi; }
run() { bash "$S" "$@" 2>/dev/null | head -1; }

mkdir -p "$PROBE"
trap 'rm -rf "$PROBE"' EXIT

for s in explore propose design implement verify archive; do
  printf '<!-- stage: %s -->\n' "$s" > "$PROBE/stage.md"
  eq "阶段 $s → stage-$s" "$(run _smoke_stage_agent)" "stage-$s"
done

printf '<!--stage:implement-->\n' > "$PROBE/stage.md"
eq "无空格 marker" "$(run _smoke_stage_agent)" "stage-implement"

printf '<!-- stage: verify    -->\n' > "$PROBE/stage.md"
eq "行尾多余空白" "$(run _smoke_stage_agent)" "stage-verify"

{ for i in $(seq 1 30); do echo "行 $i"; done; echo '<!-- stage: design -->'; } > "$PROBE/stage.md"
eq "第 31 行 marker（旧实现会漏）" "$(run _smoke_stage_agent)" "stage-design"

printf '<!-- stage: verify -->\n<!-- stage: explore -->\n' > "$PROBE/stage.md"
eq "多条 marker 判非法" "$(run _smoke_stage_agent)" "general-purpose"

printf '' > "$PROBE/stage.md"
eq "空 stage.md" "$(run _smoke_stage_agent)" "general-purpose"

printf '<!-- stage: bogus -->\n' > "$PROBE/stage.md"
eq "非法取值" "$(run _smoke_stage_agent)" "general-purpose"

printf '<!-- stage: VERIFY -->\n' > "$PROBE/stage.md"
eq "大写取值归一化" "$(run _smoke_stage_agent)" "stage-verify"

eq "不存在的 change" "$(run no_such_change_xyz)" "general-purpose"
eq "无参数" "$(run)" "general-purpose"

printf '<!-- stage: verify -->\n' > "$PROBE/stage.md"
case "$(bash "$S" _smoke_stage_agent --cheap --explain 2>/dev/null)" in
  stage-verify*) ok "cheap 不改路由（仍 stage-verify）" ;;
  *) bad "cheap 改掉路由了（review R-3 复发）" ;;
esac

printf '<!-- stage: explore -->\n' > "$PROBE/stage.md"
bash "$S" _smoke_stage_agent >/dev/null 2>&1
eq "退出码恒为 0" "$?" "0"

emit() { printf '<!-- stage: bogus -->\n' > "$PROBE/stage.md"; bash "$S" _smoke_stage_agent >/dev/null 2>&1; echo $?; }
eq "非法取值也 exit 0（fail-open）" "$(emit)" "0"

# 六档 frontmatter 漂移防护：name 必须等于文件名，且必须有 model/thinking
for f in .pi/agents/stage-*.md; do
  base="$(basename "$f" .md)"
  grep -q "^name: $base$" "$f" && ok "$base frontmatter name" || bad "$base 缺 name（会静默不注册）"
  grep -q '^model:' "$f" && ok "$base 有 model" || bad "$base 缺 model"
  grep -q '^thinking:' "$f" && ok "$base 有 thinking" || bad "$base 缺 thinking"
done

# 权限收窄：explore 必须无 write/edit/bash；verify 必须无 edit
if grep -qE '^tools:.*(write|edit|bash)' .pi/agents/stage-explore.md; then
  bad "stage-explore 混入了写权限"
else ok "stage-explore 真只读（无 write/edit/bash）"; fi
if grep -qE '^tools:.*(^|, )edit' .pi/agents/stage-verify.md; then
  bad "stage-verify 混入了 edit"
else ok "stage-verify 无 edit"; fi

printf '\n== stage-agent smoke: %d 通过 / %d 失败 ==\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
