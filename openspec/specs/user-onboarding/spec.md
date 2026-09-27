# user-onboarding

## Purpose

首次使用引导流程，帮助新用户快速了解 Syntopica 的核心功能和操作路径。包括分步引导（guided tour）、空状态引导和页面级引导。

## Requirements

### Requirement: First-run detection
The system SHALL detect first-time users by checking the absence of the `syntopica_onboarding_complete` key in localStorage. When the key does not exist, the system MUST treat the user as a first-time user and set `isFirstRun` to true.

#### Scenario: First visit detected
- **WHEN** a user loads the application and `syntopica_onboarding_complete` is not present in localStorage
- **THEN** the system identifies the user as a first-time user and `isFirstRun` is true

#### Scenario: Returning user detected
- **WHEN** a user loads the application and `syntopica_onboarding_complete` is `"true"` in localStorage
- **THEN** the system identifies the user as a returning user and `isFirstRun` is false

### Requirement: Guided tour auto-start on first run
The system SHALL automatically start the guided tour when a first-time user visits the application. The tour MUST begin after the main layout has fully mounted (sidebar, header, and navigation elements are present in the DOM).

#### Scenario: Tour starts automatically for first-time user
- **WHEN** a first-time user loads the application and the main layout is fully mounted
- **THEN** the guided tour starts automatically, showing the welcome step

#### Scenario: Tour does not start for returning user
- **WHEN** a returning user loads the application
- **THEN** no guided tour is triggered automatically

### Requirement: Guided tour steps
The system SHALL present a guided tour consisting of exactly 5 sequential steps. Each step MUST highlight a specific UI element using a `data-onboarding` attribute selector and display a popover with a title and description in Chinese. The steps SHALL be:

1. Welcome overlay (no element highlight) — "欢迎使用 Syntopica"
2. Sidebar feed/categories area (`data-onboarding="sidebar-feeds"`) — feed management introduction
3. Topic graph navigation button (`data-onboarding="nav-topic-graph"`) — topic graph introduction
4. Tag management navigation button (`data-onboarding="nav-tags"`) — tag management introduction
5. Watched tags section (`data-onboarding="watched-tags"`) — personalized feed introduction

#### Scenario: Tour displays all 5 steps in sequence
- **WHEN** the guided tour is active
- **THEN** the system displays 5 steps in order, each highlighting the corresponding `data-onboarding` element with a popover containing Chinese title and description

#### Scenario: Tour step skips missing DOM element
- **WHEN** a tour step targets a `data-onboarding` element that is not present in the DOM (e.g., sidebar is collapsed, hiding `.categories` / `.watched-tags-section` behind `v-if="!sidebarCollapsed"`)
- **THEN** the system skips that step before passing steps to driver.js (driver.js v1 does not natively skip missing elements — the composable MUST pre-filter steps via `document.querySelector` after `await nextTick()`, dropping any step whose selector returns null), and proceeds to the next available step

### Requirement: Guided tour navigation
The system SHALL provide Next, Previous, and Skip controls on each tour step. The user MUST be able to advance through all steps, go back to a previous step, or dismiss the tour entirely at any point.

#### Scenario: User advances through tour steps
- **WHEN** the user clicks "Next" on any non-final tour step
- **THEN** the tour advances to the next step and highlights the corresponding element

#### Scenario: User goes back to previous step
- **WHEN** the user clicks "Previous" on any non-first tour step
- **THEN** the tour returns to the previous step and highlights its element

#### Scenario: User completes the final step
- **WHEN** the user clicks "Next" (or "Done") on the final tour step
- **THEN** the tour ends and `syntopica_onboarding_complete` is set to `"true"` in localStorage

### Requirement: Guided tour dismissal
The system SHALL allow the user to dismiss the tour at any step by clicking a "Skip" button or clicking outside the popover. Upon dismissal, the system MUST set `syntopica_onboarding_complete` to `"true"` in localStorage.

#### Scenario: User skips the tour
- **WHEN** the user clicks "Skip" on any tour step
- **THEN** the tour ends immediately and `syntopica_onboarding_complete` is set to `"true"` in localStorage

#### Scenario: User clicks outside tour popover
- **WHEN** the user clicks outside the highlighted area or popover during the tour
- **THEN** the tour ends and `syntopica_onboarding_complete` is set to `"true"` in localStorage

### Requirement: Guided tour re-trigger from header toolbar
The system SHALL provide a "新手引导" (Onboarding Guide) icon button in the homepage top toolbar (`.header-right` in `AppHeaderView.vue`), at the same level as the theme toggle button. When clicked, the system MUST start the guided tour via `useOnboarding().startTour()` without a page reload.

#### Scenario: User triggers tour replay from header toolbar
- **WHEN** the user clicks the "新手引导" button in the top toolbar
- **THEN** the guided tour starts immediately (no page reload, no localStorage mutation required)

### Requirement: Empty state guide for feeds
When the user has no RSS feeds and no feed categories, the system SHALL display a `FeedEmptyGuide` component in the sidebar or main content area. The guide MUST include a message encouraging the user to add their first RSS feed and an action button to trigger the feed addition dialog.

#### Scenario: Feed empty state displayed
- **WHEN** the user's feed list is empty and no categories exist
- **THEN** the system displays `FeedEmptyGuide` with a Chinese message and an "添加 RSS 源" action button

#### Scenario: Feed empty state hidden when feeds exist
- **WHEN** the user has at least one feed or category
- **THEN** the `FeedEmptyGuide` component is not displayed

### Requirement: Empty state guide for topic graph
When the topic graph has no data (no tags or no tag relationships), the system SHALL display a `TopicGraphEmptyGuide` component explaining why the graph is empty and suggesting actions (e.g., wait for content crawling and tagging).

#### Scenario: Topic graph empty state displayed
- **WHEN** the topic graph has no tag data to render
- **THEN** the system displays `TopicGraphEmptyGuide` with an explanation and suggested actions

#### Scenario: Topic graph empty state hidden when data exists
- **WHEN** the topic graph has tags and relationships to display
- **THEN** the `TopicGraphEmptyGuide` component is not displayed

### Requirement: Empty state guide for daily reports
When no daily reports exist, the system SHALL display a `DailyReportEmptyGuide` component explaining that daily reports require accumulated data and time.

#### Scenario: Daily report empty state displayed
- **WHEN** the daily report list is empty
- **THEN** the system displays `DailyReportEmptyGuide` with an explanation about data accumulation

#### Scenario: Daily report empty state hidden when reports exist
- **WHEN** at least one daily report is available
- **THEN** the `DailyReportEmptyGuide` component is not displayed

### Requirement: prefers-reduced-motion accessibility
The system SHALL respect the `prefers-reduced-motion: reduce` media query. When reduced motion is preferred, the guided tour highlight animations MUST be disabled (instant transitions only).

#### Scenario: Tour with reduced motion preference
- **WHEN** the user's system has `prefers-reduced-motion: reduce` enabled and the guided tour is active
- **THEN** all tour highlight transitions and popover animations are disabled, using instant state changes instead

### Requirement: useOnboarding composable API
The system SHALL provide a `useOnboarding` Vue composable exposing: `isFirstRun` (computed boolean), `isTourActive` (computed boolean), `startTour()` (function), `dismissTour()` (function), and `resetOnboarding()` (function). The composable MUST guard all `document` / `localStorage` / driver.js access with an `import.meta.client` check, so that it remains safe if SSR is ever enabled.

#### Scenario: Composable provides reactive state
- **WHEN** a Vue component calls `useOnboarding()`
- **THEN** the composable returns `isFirstRun` and `isTourActive` as reactive computed refs, and all functions are callable

#### Scenario: Composable client-guard (defensive, current app is SPA)
- **WHEN** `useOnboarding()` is called and `import.meta.client` is `false` (e.g., if SSR is enabled in the future, or during a Vitest run with the guard mocked off)
- **THEN** no driver.js instance is created and no localStorage access occurs; `isFirstRun` defaults to false and `isTourActive` defaults to false
- **NOTE** The Syntopica frontend currently runs with `ssr: false` (`front/nuxt.config.ts`), so this guard is a defensive reservation, not a response to an active SSR risk.

### Requirement: Tags page guided tour
The system SHALL present a separate guided tour on the tag management page (`/tags`, `TagsPage.vue`) covering the core semantic-board workflow. The tour SHALL consist of sequential steps highlighting: the board list (`data-onboarding="tags-board-list"`), the three content tabs (`data-onboarding="tags-content-tabs"`), the board actions group (`data-onboarding="tags-board-actions"`), and the add-board button (`data-onboarding="tags-add-board"`). The tour MUST use an independent completion key (`syntopica_onboarding_tags_complete`) so it does not interfere with the home tour, and MUST reuse the same `useOnboarding` driver-instance management, `prefers-reduced-motion` detection, and missing-element pre-filtering.

#### Scenario: Tags tour auto-starts on first visit
- **WHEN** a first-time visitor loads `/tags` and `syntopica_onboarding_tags_complete` is absent
- **THEN** the tags guided tour starts automatically after the page mounts

#### Scenario: Tags tour re-trigger from toolbar
- **WHEN** the user clicks the guide icon button in the `TagsPage.vue` top toolbar (next to `ThemeToggle`)
- **THEN** the tags guided tour starts immediately (no page reload)

#### Scenario: Independent completion markers
- **WHEN** the home tour is completed and the user visits `/tags`
- **THEN** the tags tour still runs (its `syntopica_onboarding_tags_complete` key is independent) until it is also completed

### Requirement: Settings page guided tour
The system SHALL present a separate guided tour on the settings page (`/settings`, `SettingsWorkspace.vue`) covering the core configuration chain (data source → AI capability → scheduled tasks). The tour SHALL highlight: the grouped sidebar nav (`data-onboarding="settings-nav"`), the feeds section (`data-onboarding="settings-nav-feeds"`), the AI providers section (`data-onboarding="settings-nav-ai-providers"`), and the runtime status composite section (`data-onboarding="settings-nav-runtime-status"`). Tour steps SHALL reference only section keys that exist in the grouped navigation; the legacy `settings-nav-schedulers` anchor SHALL NOT be referenced. The tour MUST use an independent completion key (`syntopica_onboarding_settings_complete`) and reuse the same `useOnboarding` driver management, `prefers-reduced-motion` detection, and missing-element pre-filtering.

#### Scenario: Settings tour auto-starts on first visit
- **WHEN** a first-time visitor loads `/settings` and `syntopica_onboarding_settings_complete` is absent
- **THEN** the settings guided tour starts automatically after the page mounts

#### Scenario: Settings tour re-trigger from header
- **WHEN** the user clicks the guide icon button in the `SettingsWorkspace.vue` header (next to the theme toggle)
- **THEN** the settings guided tour starts immediately (no page reload)

#### Scenario: Settings tour independent of other tours
- **WHEN** the home and tags tours are completed and the user visits `/settings`
- **THEN** the settings tour still runs (its `syntopica_onboarding_settings_complete` key is independent) until it is also completed

#### Scenario: Settings tour anchors resolve on grouped navigation
- **WHEN** the settings tour runs
- **THEN** every tour step anchor resolves to an existing element in the grouped sidebar (no step is silently skipped due to a missing `data-onboarding` anchor)
