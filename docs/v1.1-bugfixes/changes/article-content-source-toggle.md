# Article Content Source Toggle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let readers switch between feed-provided original content and Firecrawl-captured full content in the article detail view.

**Architecture:** Keep the feature local to the article detail screen. Add a small source toggle in `ArticleContent.vue`, backed by a tiny pure helper so source availability and default selection can be tested without mounting the whole component.

**Tech Stack:** Nuxt 4, Vue 3, TypeScript, Vitest

---

### Task 1: Add source-selection helper tests

**Files:**
- Create: `front/app/utils/articleContentSource.test.ts`
- Create: `front/app/utils/articleContentSource.ts`

**Step 1: Write the failing test**

Cover these cases:
- both Firecrawl and original content exist -> expose both options and default to `firecrawl`
- only original content exists -> only expose `original`
- only Firecrawl content exists -> only expose `firecrawl`

**Step 2: Run test to verify it fails**

Run: `pnpm test:unit app/utils/articleContentSource.test.ts`

**Step 3: Write minimal implementation**

Implement a helper that returns:
- available sources
- default source
- rendered content for a chosen source

**Step 4: Run test to verify it passes**

Run: `pnpm test:unit app/utils/articleContentSource.test.ts`

### Task 2: Wire the toggle into article detail

**Files:**
- Modify: `front/app/components/article/ArticleContent.vue`

**Step 1: Add local source state**

Use the helper to compute available sources and preferred default.

**Step 2: Add the toggle UI**

Show it only when both sources exist.

**Step 3: Switch displayed content**

Make the article body and description guard read from the selected source.

**Step 4: Reset state when article changes**

Ensure navigation between articles picks the right default each time.

### Task 3: Verify the feature

**Files:**
- Test: `front/app/utils/articleContentSource.test.ts`
- Check: `front/app/components/article/ArticleContent.vue`

**Step 1: Run focused unit tests**

Run: `pnpm test:unit app/utils/articleContentSource.test.ts`

**Step 2: Run typecheck**

Run: `pnpm exec nuxi typecheck`

Expected: feature files stay clean; if unrelated repo errors remain, report them separately.
