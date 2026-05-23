## ADDED Requirements

### Requirement: Article detail API returns tag watch status

The `GET /api/articles/:article_id` endpoint SHALL include `id` and `is_watched` fields in each tag object within the `tags` array of the response.

#### Scenario: Tag with watched status returned
- **WHEN** a user requests an article that has tags, and some of those tags are watched
- **THEN** the response `tags` array SHALL contain tag objects with `id` (integer), `is_watched` (boolean), and `article_count` (integer) fields

#### Scenario: Tag without watched status returned
- **WHEN** a user requests an article that has tags, and none of those tags are watched
- **THEN** the response `tags` array SHALL contain tag objects with `is_watched: false`

### Requirement: Article tag list supports watch toggle UI

The `ArticleTagList` component SHALL support an optional watch toggle mode. When `showWatch` prop is `true`, each tag SHALL display a heart icon indicating its watched status, and clicking the icon SHALL emit a `watchToggle` event.

#### Scenario: Show watch icons when enabled
- **WHEN** `ArticleTagList` receives `showWatch: true` and a `tags` array with `isWatched` fields
- **THEN** each tag pill SHALL render a clickable heart icon: filled heart (`mdi:heart`) when `isWatched` is true, outlined heart (`mdi:heart-outline`) when false

#### Scenario: No watch icons when disabled
- **WHEN** `ArticleTagList` receives `showWatch: false` (default)
- **THEN** no heart icons SHALL be rendered on any tag

#### Scenario: Click heart icon to toggle watch
- **WHEN** a user clicks the heart icon on a tag
- **THEN** the component SHALL emit a `watchToggle` event with the tag's `id` and `slug` as payload

### Requirement: Article detail page handles watch/unwatch actions

The `ArticleContentView` component SHALL handle `watchToggle` events from `ArticleTagList` by calling the watch/unwatch API and updating the article's local tag data optimistically.

#### Scenario: Watch a previously unwatched tag
- **WHEN** user clicks an outlined heart on a tag
- **THEN** the UI SHALL immediately show a filled heart, and a `POST /api/topic-tags/:tag_id/watch` request SHALL be sent

#### Scenario: Unwatch a previously watched tag
- **WHEN** user clicks a filled heart on a tag
- **THEN** the UI SHALL immediately show an outlined heart, and a `POST /api/topic-tags/:tag_id/unwatch` request SHALL be sent

#### Scenario: API failure rolls back optimistic update
- **WHEN** the watch/unwatch API request fails
- **THEN** the heart icon SHALL revert to its previous state

#### Scenario: Tag without ID does not show watch icon
- **WHEN** a tag object lacks `id` field
- **THEN** no watch heart icon SHALL be displayed for that tag
