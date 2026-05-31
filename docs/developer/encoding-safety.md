# Encoding Safety

Date: 2026-03-07

Problem
- On Windows PowerShell, shell-based rewrites can corrupt non-ASCII text.
- Common signs include broken UI copy, broken comments, or unexpected replacement characters.

High-risk operations
- `Set-Content`, `Out-File`, `>` or `>>` on `.vue`, `.ts`, `.go`, `.md`, `.py`
- `git show > file` on Windows
- Bulk rewrites of files that already contain non-ASCII text

Required checks
1. Prefer UTF-8 without BOM.
2. Re-open touched files immediately after writing.
3. Run a repository search for suspicious text patterns after editing.
4. If corruption appears, stop patching line-by-line and rewrite the whole file in UTF-8.

Recommended commands
```bash
rg -n "broken UI copy|unexpected replacement" <paths>
Get-Content <file>
```

Incident references
- `front/app/components/article/ArticleContent.vue`
- `backend-go/internal/schedulers/auto_summary.go`
- `backend-go/internal/schedulers/content_completion.go`
- `AGENTS.md`
