## ADDED Requirements

### Requirement: Acceptance test project structure
The system SHALL provide a `tests/acceptance/` directory with uv-managed Python project containing pytest and playwright dependencies. The project SHALL have a `pyproject.toml` with `pytest>=8.0`, `playwright>=1.40`, and `requests>=2.31` as dependencies.

#### Scenario: uv project initializes correctly
- **WHEN** developer runs `cd tests/acceptance && uv sync`
- **THEN** a `.venv` directory SHALL be created with pytest, playwright, and requests installed

#### Scenario: Playwright browsers installed
- **WHEN** developer runs `uv run playwright install chromium`
- **THEN** Chromium browser SHALL be available for acceptance tests

### Requirement: Environment readiness check
The system SHALL provide a session-scoped pytest fixture that verifies both Go backend (localhost:5000) and Nuxt frontend (localhost:3000) are running before any tests execute. If either service is unreachable, pytest SHALL exit immediately with a clear message indicating which service is down.

#### Scenario: Both services running
- **WHEN** backend responds 200 on GET /api/feeds and frontend responds on GET /
- **THEN** tests SHALL proceed normally

#### Scenario: Backend not running
- **WHEN** backend does not respond on localhost:5000 within 5 seconds
- **THEN** pytest SHALL exit with message "后端未运行在 localhost:5000"

#### Scenario: Frontend not running
- **WHEN** frontend does not respond on localhost:3000 within 5 seconds
- **THEN** pytest SHALL exit with message "前端未运行在 localhost:3000"

### Requirement: Playwright browser fixtures
The system SHALL provide pytest fixtures for Playwright browser, context, and page objects. Browser SHALL launch in headless mode by default (overridable via env var). Context SHALL set viewport to 1280x720.

#### Scenario: Page fixture provides navigable browser page
- **WHEN** a test uses the `page` fixture
- **THEN** the page SHALL be a Playwright Page object with viewport 1280x720 pointing to localhost:3000

### Requirement: API client helper
The system SHALL provide an API client helper class that wraps HTTP requests to the Go backend. The helper SHALL support GET, POST, PUT, DELETE methods with automatic JSON parsing and error handling.

#### Scenario: API client creates sector successfully
- **WHEN** helper calls POST /api/narratives/board-concepts/sectors with valid data
- **THEN** response SHALL be parsed as JSON dict with `success: true`

#### Scenario: API client handles backend error
- **WHEN** backend returns HTTP 500
- **THEN** helper SHALL raise an exception with status code and response body

### Requirement: Browser navigation helpers
The system SHALL provide helper functions for common page navigation patterns: `navigate_to_tags(page)` navigates to `/tags` and waits for network idle.

#### Scenario: Navigate to tags page
- **WHEN** `navigate_to_tags(page)` is called
- **THEN** browser SHALL be at URL `/tags` with network idle state

### Requirement: Selector constants
The system SHALL provide a centralized selector dictionary in `helpers/selectors.py` containing all CSS class names and text selectors used by UI tests. Tests SHALL NOT hardcode selectors inline.

#### Scenario: Sector list selectors defined
- **WHEN** a test needs to locate the sector list
- **THEN** it SHALL use `SECTOR_LIST["container"]` from selectors module

#### Scenario: Selector update requires single file change
- **WHEN** a CSS class name changes in the Vue component
- **THEN** only `helpers/selectors.py` SHALL need updating, no test files

### Requirement: Change-based test organization
Tests SHALL be organized under `tests/acceptance/changes/<change-name>/` directories. Each change directory SHALL contain a `conftest.py` for change-specific fixtures. Test files SHALL follow `test_story_XX_<name>.py` naming where `00` prefix denotes API tests and `01+` denotes UI tests.

#### Scenario: Running all acceptance tests
- **WHEN** developer runs `cd tests/acceptance && uv run pytest changes/ -v`
- **THEN** all change directories SHALL be discovered and tests run in order

#### Scenario: Running single change tests
- **WHEN** developer runs `uv run pytest changes/unify-tag-hierarchy/ -v`
- **THEN** only that change's tests SHALL run

#### Scenario: Running single story
- **WHEN** developer runs `uv run pytest changes/unify-tag-hierarchy/ -k "story_01" -v`
- **THEN** only matching story files SHALL run
