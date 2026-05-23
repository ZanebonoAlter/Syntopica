## 1. Backend — Extend tag response with watch status

- [x] 1.1 Add `IsWatched` (`bool`) and `ArticleCount` (`int`) fields to `topictypes.TopicTag` struct in `backend-go/internal/domain/topictypes/types.go`
- [x] 1.2 Update `GetArticleTags` in `backend-go/internal/domain/topicextraction/article_tagger.go` to populate `ID`, `IsWatched`, and `ArticleCount` when mapping from `models.TopicTag`
- [x] 1.3 Run `go build ./...` in `backend-go/` to verify compilation
- [x] 1.4 Run `go test ./...` in `backend-go/` to verify no regressions (pre-existing failures unrelated)

## 2. Frontend — Extend ArticleTag type and data mapping

- [x] 2.1 Add `id?: number` and `isWatched?: boolean` to `ArticleTag` interface in `front/app/types/article.ts`
- [x] 2.2 Update `normalizeArticleTags` in `front/app/features/articles/utils/normalizeArticle.ts` to map `id` and `is_watched` from API response
- [x] 3.1 Add `showWatch` prop and `watchToggle` emit to `ArticleTagList.vue`
- [x] 3.2 Render heart icon (`mdi:heart` / `mdi:heart-outline`) per tag when `showWatch` is `true` and tag has `id`
- [x] 3.3 Style the heart icon inline within tag pill, matching existing tag pill style
- [x] 3.4 Emit `watchToggle` event with `{ id: number, slug: string }` payload on icon click
- [x] 4.1 Import `useWatchedTagsApi` in `ArticleContentView.vue`
- [x] 4.2 Pass `show-watch` prop to `ArticleTagList` instances in the template
- [x] 4.3 Implement `handleTagWatchToggle` with optimistic update and API call (watch/unwatch)
- [x] 4.4 Handle API failure by reverting optimistic update
- [x] 4.5 Wire `@watch-toggle` event to handler

## 5. Verification

- [x] 5.1 Run `pnpm exec nuxi typecheck` in `front/` to verify TypeScript
- [x] 5.2 Run `pnpm build` in `front/` to verify production build
- [ ] 5.3 Manual smoke test: open article detail, verify heart icons show, click to toggle watch/unwatch
