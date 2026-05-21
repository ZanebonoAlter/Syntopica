## ADDED Requirements

### Requirement: /tags page loads with correct layout
The UI test SHALL verify that navigating to `/tags` renders the three-area layout: top bar with category switcher, left panel with sector list, right panel with hierarchy tree, and bottom bar with pending changes button.

#### Scenario: Page loads without crash
- **WHEN** user navigates to /tags and waits for network idle
- **THEN** page SHALL render without errors and display text "标签管理"

#### Scenario: Category switcher visible
- **WHEN** page has loaded
- **THEN** three category buttons SHALL be visible with text "事件", "人物", "关键词"

#### Scenario: Three-area layout visible
- **WHEN** page has loaded
- **THEN** left panel (.sector-list or equivalent) SHALL be visible
- **THEN** right panel (hierarchy tree area) SHALL be visible
- **THEN** bottom bar (.tags-bottombar) SHALL be visible

#### Scenario: Default category is event
- **WHEN** page has loaded
- **THEN** "事件" button SHALL have active styling

### Requirement: Sector list displays and responds to interaction
The UI test SHALL verify that the sector list shows existing sectors with source icons and tag counts, responds to category switching, and highlights selected sectors.

#### Scenario: Sector list shows items
- **WHEN** page loads with event category that has sectors
- **THEN** sector list SHALL show items with labels and tag counts

#### Scenario: Category switch updates list
- **WHEN** user clicks "人物" category button
- **THEN** sector list SHALL update to show person-category sectors
- **THEN** "人物" button SHALL have active styling

#### Scenario: "全部" option exists
- **WHEN** sector list is visible
- **THEN** an "全部" option SHALL be visible in the sector list

### Requirement: Manual sector creation via UI
The UI test SHALL verify the full manual sector creation flow: click add → fill dialog → confirm → verify sector appears in list.

#### Scenario: Create sector with label only
- **WHEN** user clicks "添加板块" button
- **THEN** a dialog SHALL appear with title "添加板块"
- **WHEN** user types a label in the name input and clicks "确认添加"
- **THEN** dialog SHALL close and the new sector SHALL appear in the sector list

#### Scenario: Empty label prevents creation
- **WHEN** user clicks "添加板块" and the name input is empty
- **THEN** "确认添加" button SHALL be disabled

### Requirement: Sector selection filters hierarchy tree
The UI test SHALL verify that clicking a sector filters the right panel hierarchy tree, and clicking "全部" restores the full view.

#### Scenario: Sector click filters tree
- **WHEN** user clicks a specific sector in the list
- **THEN** that sector item SHALL have active styling
- **THEN** the right panel hierarchy tree SHALL show only that sector's tags

#### Scenario: "全部" restores full view
- **WHEN** user clicks "全部" in the sector list
- **THEN** the hierarchy tree SHALL show the full category tree
- **THEN** no single sector SHALL be highlighted

#### Scenario: No sectors shows empty state
- **WHEN** selected category has no sectors
- **THEN** sector list SHALL show empty state message

### Requirement: Template modification triggers rebuild flow
The UI test SHALL verify the template settings dialog workflow: open dialog → modify level → save → impact confirmation → rebuild progress.

#### Scenario: Template dialog opens with level list
- **WHEN** user clicks the settings button (gear icon)
- **THEN** a dialog SHALL appear showing current hierarchy levels with name, max_children, and is_leaf fields

#### Scenario: Modify and save template
- **WHEN** user modifies a level name and clicks "保存"
- **THEN** an impact confirmation overlay SHALL appear showing affected tag count and estimated time
- **WHEN** user clicks "确认重建"
- **THEN** the dialog SHALL close and the bottom bar SHALL show rebuild progress

#### Scenario: Rebuild progress visible
- **WHEN** rebuild is processing
- **THEN** bottom bar SHALL show a progress indicator with processed/total count

### Requirement: Pending changes badge and approval panel
The UI test SHALL verify the pending changes badge in the bottom bar and the approval panel interaction.

#### Scenario: Pending changes badge shows count
- **WHEN** pending changes exist
- **THEN** bottom bar SHALL show "待确认变更" button with a count badge

#### Scenario: Approval panel opens
- **WHEN** user clicks "待确认变更" button
- **THEN** a pending change panel SHALL become visible

#### Scenario: No pending changes
- **WHEN** no pending changes exist
- **THEN** badge count SHALL be 0 or not shown
