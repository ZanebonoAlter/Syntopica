## MODIFIED Requirements

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
