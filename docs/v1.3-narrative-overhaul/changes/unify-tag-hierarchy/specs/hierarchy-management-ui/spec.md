## ADDED Requirements

### Requirement: /tags page route and layout
The system SHALL provide a `/tags` page with a two-panel layout: left panel for Sector management, right panel for hierarchy tree view. A bottom bar SHALL display rebuild progress and pending change counts. A category switcher (event/person/keyword) SHALL be in the top bar.

#### Scenario: User navigates to /tags
- **WHEN** user navigates to /tags
- **THEN** the page SHALL load with event category selected by default, showing Sector list on the left and hierarchy tree on the right

### Requirement: Sector list panel
The left panel SHALL display all Sectors for the selected category, each showing: label, source icon (🔒 manual / 🤖 LLM / ⚡ auto), Tag count. Actions: "添加板块" (manual mode), "LLM 重新生成" (LLM mode). Clicking a Sector SHALL filter the right panel to show only that Sector's hierarchy tree. A "全部" option SHALL show the full category tree.

#### Scenario: Sector list displays source icons
- **WHEN** category "event" has 3 Sectors: "AI产品" (manual), "AI商业" (LLM), "技术架构" (auto)
- **THEN** the Sector list SHALL show 🔒 for "AI产品", 🤖 for "AI商业", ⚡ for "技术架构"

#### Scenario: Clicking Sector filters hierarchy
- **WHEN** user clicks Sector "AI产品"
- **THEN** the right panel hierarchy tree SHALL show only Tags and Nodes with concept_id matching "AI产品"

#### Scenario: Sector deletion
- **WHEN** user clicks delete on a Sector
- **THEN** for protected Sectors, a confirmation dialog SHALL appear; for non-protected, single confirmation SHALL suffice; upon confirmation, the Sector and all its Tags' concept_id references SHALL be cleared

### Requirement: Hierarchy tree panel
The right panel SHALL display the hierarchy tree for the selected context (Sector or full category). Each Node SHALL be expandable to show children. Leaf Tags SHALL show label and article count. Nodes SHALL support: inline rename, detach child, reassign child to another Node. An "未归属" section SHALL list unplaced Tags.

#### Scenario: Tree renders Node hierarchy
- **WHEN** Sector "AI产品" has Node "AI产品发布" with children "GPT-5发布" and "Claude 4发布"
- **THEN** the tree SHALL render "AI产品发布" as expandable parent with two leaf children

#### Scenario: Inline Node rename
- **WHEN** user double-clicks Node "AI产品发布" and types "AI产品动态" then presses Enter
- **THEN** the system SHALL call PUT /api/topic-tags/:id/abstract-name with the new name

#### Scenario: Unplaced Tags section
- **WHEN** 5 Tags in category "event" have no parent relation and no concept_id
- **THEN** an "未归属" section SHALL list these 5 Tags at the bottom of the tree

### Requirement: Template settings dialog
A settings button in the top bar SHALL open a modal for editing the selected category's HierarchyTemplate. The modal SHALL display Level definitions (name, max_children, is_leaf) with controls to add, remove, or reorder Levels. On save, the modal SHALL display impact summary (affected Tag count + estimated rebuild time) and require user confirmation.

#### Scenario: Template editor renders levels
- **WHEN** user opens template settings for category "event"
- **THEN** the modal SHALL show Level 1 "事件类型" max_children=20, Level 2 "事件主体" max_children=50, Level 3 "具体事件" is_leaf=true

#### Scenario: Add new level
- **WHEN** user clicks "+ 添加层" and enters name "事件阶段"
- **THEN** a new Level SHALL be added at the bottom (before leaf level if applicable)

#### Scenario: Save with impact preview
- **WHEN** user modifies template and clicks save
- **THEN** the system SHALL display "此变更将影响 247 个标签，预计重建耗时 15-20 分钟" with [确认重建] [取消] buttons

### Requirement: Rebuild progress display
During an active rebuild, the bottom bar SHALL display a progress bar with: processed/total count, estimated remaining time, and current status. The progress SHALL update in real-time via WebSocket.

#### Scenario: Progress bar during rebuild
- **WHEN** rebuild_job for "event" is running with 187/247 Tags processed
- **THEN** the bottom bar SHALL show [████████░░░░] 187/247, estimated remaining: 5 分钟

#### Scenario: Rebuild completed notification
- **WHEN** rebuild_job completes
- **THEN** the bottom bar SHALL show "重建完成: 247 标签已处理, 0 失败" with a dismiss button

### Requirement: PendingChange batch approval
The bottom bar SHALL show a count of pending hierarchy changes. Clicking it SHALL open a panel listing changes grouped by Sector/Category. Each change SHALL show: tag label, current parent, proposed action, reason. Users SHALL be able to "全部确认" or select individual changes to approve. Approved changes SHALL be executed and results displayed.

#### Scenario: PendingChange count displayed
- **WHEN** 12 hierarchy_pending_changes exist with status='pending'
- **THEN** the bottom bar SHALL show "待确认变更: 12 条 [查看]"

#### Scenario: Batch approve all
- **WHEN** user clicks "全部确认" on 12 pending changes
- **THEN** all 12 changes SHALL be executed and the list SHALL clear

### Requirement: TopicGraphPage hierarchy tab removal
The hierarchy tab SHALL be removed from TopicGraphPage (/topic-graph). The page SHALL only contain graph and narrative tabs. All hierarchy management SHALL be accessible from /tags.

#### Scenario: TopicGraphPage has no hierarchy tab
- **WHEN** user navigates to /topic-graph
- **THEN** only "图谱" and "叙事" tabs SHALL be visible

### Requirement: GlobalSettingsDialog hierarchy tab removal
The hierarchy tab SHALL be removed from GlobalSettingsDialog. Template editing, pending changes, and rebuild trigger SHALL be accessible from /tags.

#### Scenario: GlobalSettings has no hierarchy tab
- **WHEN** user opens GlobalSettingsDialog
- **THEN** no "层级" tab SHALL be present

### Requirement: Tag click opens timeline in topic graph
When user clicks a tag on the topic graph page (via hotspot tag badge or graph canvas node), the floating timeline panel SHALL automatically open to display articles related to that tag. The data loading pipeline (loadAggregatedArticles) is already in place; only the timeline panel visibility toggle is missing.

#### Scenario: Hotspot badge click opens timeline
- **WHEN** user clicks a hotspot tag badge on the topic graph page
- **THEN** the timeline panel SHALL become visible (timelineOpen = true) and SHALL show aggregated articles for that tag

#### Scenario: Graph node click opens timeline
- **WHEN** user clicks a graph canvas node representing a tag
- **THEN** the timeline panel SHALL become visible and SHALL show articles for that tag

#### Scenario: Timeline already open stays open
- **WHEN** user clicks a different tag while timeline is already open
- **THEN** the timeline SHALL update to show articles for the newly selected tag without closing and reopening

### Requirement: Sector click shows filtered hierarchy and narrative
When user clicks a Sector in the /tags page left panel, the right panel SHALL switch to show: (a) only that Sector's hierarchy tree (Nodes and Tags with matching concept_id), and (b) a narrative/article timeline section showing recent articles for Tags within that Sector. This provides lightweight constraint-effect preview without triggering a full rebuild. A "全部" option in the Sector list SHALL restore the full category view.

#### Scenario: Clicking Sector filters hierarchy tree
- **WHEN** user clicks Sector "AI产品" which has 12 Tags across 3 Nodes
- **THEN** the hierarchy tree SHALL show only Nodes and Tags belonging to "AI产品"

#### Scenario: Sector view shows narrative timeline
- **WHEN** user clicks Sector "AI产品"
- **THEN** the right panel SHALL include a narrative section showing recent articles grouped by date for Tags within that Sector

#### Scenario: Clicking "全部" restores full view
- **WHEN** user clicks "全部" in the Sector list
- **THEN** the hierarchy tree SHALL show the full category tree (all Sectors), and the narrative section SHALL be hidden or show cross-sector content

#### Scenario: Sector with no Tags shows empty state
- **WHEN** user clicks a Sector that has 0 Tags
- **THEN** the hierarchy tree SHALL show an empty state message, and the narrative section SHALL show "暂无内容"

### Requirement: LLM sector suggestion approval panel
After LLM generates sector suggestions via "LLM 重新生成板块", the system SHALL open a dedicated approval panel (separate from PendingChangePanel) where users can review each suggested change individually. Each suggestion (keep/add/merge/split) SHALL be displayed as a card with affected Tag count and rationale. Users SHALL approve or reject individual suggestions, or approve all. Upon confirmation, the system SHALL show execution progress (which suggestions are being applied) and display execution results (sectors created/merged, Tags affected, failures). The panel SHALL remain open showing results until dismissed.

#### Scenario: Approval panel shows individual suggestions
- **WHEN** LLM returns suggestions to keep 3 Sectors, add 1 Sector "AI开源", and merge "技术架构" into "AI产品"
- **THEN** the approval panel SHALL display each suggestion as a separate card with: change type icon, Sector name(s), affected Tag count, and LLM-provided rationale

#### Scenario: User approves individual suggestion
- **WHEN** user clicks [接受] on "新增: AI开源" card
- **THEN** the card SHALL show an approved state (✓), and the "全部批准" button SHALL update to show progress (e.g., "执行: 1/2 已批准")

#### Scenario: User rejects individual suggestion
- **WHEN** user clicks [拒绝] on "合并: 技术架构→AI产品" card
- **THEN** the card SHALL show a rejected state (✗) and this change SHALL NOT be included in the execution batch

#### Scenario: Approve all and execute
- **WHEN** user clicks "全部批准" or confirms after reviewing all suggestions
- **THEN** the panel SHALL show execution progress (e.g., "正在执行 2 项变更..."), then display results: "执行完成: 新增 1 板块, 拆分 1 板块, 受影响标签 22 个"

#### Scenario: Execution fails partially
- **WHEN** one suggestion fails during execution (e.g., API error)
- **THEN** the panel SHALL show "执行完成: 成功 1, 失败 1" with the failed item highlighted and an error message

#### Scenario: Panel shows execution results
- **WHEN** all suggestions have been executed (or rejected)
- **THEN** the panel SHALL display a summary: total suggestions, accepted, rejected, executed (success/fail), total Tags affected. The panel SHALL remain open until user dismisses it.
