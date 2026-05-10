# Frontend Agent Guide

遵循根 `AGENTS.md` 的所有规则。以下为前端特有差异。

## Frontend-Specific Conventions
- Always `<script setup lang="ts">` (Composition API).
- Components: PascalCase. Composables: camelCase with `use` prefix.
- Import order: Vue/Nuxt → third-party → internal → type-only.
- Use `import type` for type-only deps. Use `~` alias for app-root paths.
- Convert numeric IDs to strings at API boundary. `snake_case → camelCase` in stores/API, never in templates.
- HTTP via `ApiClient` in `app/api/client.ts`. Return `{ success, data, error, message }`.
- Files must be UTF-8. Preserve existing semicolon style per file.
- UI: editorial/magazine feel, avoid generic SaaS look.

## Anti-Patterns
- No API calls in components. No `any` types. No Options API. No `@ts-ignore`.

## Commands
```bash
pnpm install  &&  pnpm dev  &&  pnpm build
pnpm exec nuxi typecheck  &&  pnpm test:unit  &&  pnpm test:e2e
```
