package repository

import "time"

// BoardStory status values (CHECK-owned by migration 20261105_0001).
const (
	StoryStatusActive  = "active"  // 仍在演进，参与召回与热度
	StoryStatusSettled = "settled" // 人工或规则判定已收束
	StoryStatusMerged  = "merged"  // 已合并进另一条事件线（merged_into 指向存活者）
)

// BoardStory origin values.
const (
	StoryOriginRule   = "rule"   // 确定性判定（subject_key / 指纹全同 / 阶段）
	StoryOriginModel  = "model"  // LLM 兜底判定（指纹缺失或不一致的窄区间）
	StoryOriginManual = "manual" // 人工指派/合并，永不被自动判定覆盖
)

// StorySignal kind values. 本期只写 editorial（信源报道）；
// signal（社媒/讨论帖等外部信号）是预留位，写入路径未开放。
const (
	StorySignalKindEditorial = "editorial"
	StorySignalKindSignal    = "signal"
)

// BoardStory — 事件线：一条贯穿多天多个事件（DailyReportThread）的同一事件
// 聚合体（add-event-story-heat design §6.1 / specs/event-story）。
//
// 身份与热度分离：本表只承载身份（subject_key / action / object / 首末见日期），
// 热度证据在 story_signals，热度由 signals 实时计算，不落本表。
//
// 与邻居 BoardPersistentTopic 的分工：话题是跨月主题（版块下的泳道），
// 事件线是跨天事件；一条事件线可选关联首次归属话题（LaneID）。
type BoardStory struct {
	ID      uint `gorm:"primarykey" json:"id"`
	BoardID uint `gorm:"not null;index:board_stories_subject_idx,priority:1" json:"board_id"`
	// LaneID 首次归属话题（board_persistent_topics.id）；可空——归属判定
	// 不依赖话题存在，话题归档/删除不影响事件线身份。
	LaneID *uint `json:"lane_id,omitempty"`
	// 结构化事件指纹（与 event 标签同一次 LLM 调用产出）。
	// SubjectKey 是归一化实体键（如 person:trump），主体归一化的主判据；
	// Subject/Action/Object 保留原文用于展示与阶段判定。
	// string 列一律不写 size（GORM 默认 → text），与迁移 DDL 严格一致：
	// 写 size 会让 AutoMigrate 每次启动 ALTER COLUMN，并按 20261103_0002
	// 事故链在 field.Nullable 时顺带 DROP 迁移固化的 NOT NULL。
	Title      string `gorm:"not null" json:"title"`
	Subject    string `json:"subject"`
	SubjectKey string `gorm:"index:board_stories_subject_idx,priority:2" json:"subject_key"`
	Action     string `json:"action"`
	Object     string `json:"object"`
	// OccurredAt 事件发生时间（可空：不是所有事件都能定位到时刻）。
	OccurredAt *time.Time `json:"occurred_at,omitempty"`
	Status     string     `gorm:"not null;default:active" json:"status"`
	// FirstSeenDate / LastSeenDate 是事件线活跃区间（date，非时间戳），
	// 随每次新事件挂入滚动更新。
	FirstSeenDate    time.Time `gorm:"type:date;not null" json:"first_seen_date"`
	LastSeenDate     time.Time `gorm:"type:date;not null" json:"last_seen_date"`
	ThreadCount      int       `gorm:"not null;default:0" json:"thread_count"`
	ArticleCount     int       `gorm:"not null;default:0" json:"article_count"`
	ParticipantCount int       `gorm:"not null;default:0" json:"participant_count"`
	// Embedding 召回向量（title 级）；SubjectEmbedding 主体归一化兜底向量
	// （subject_key 的 embedding，cos ≥ 0.92 同义兜底）。两者均为运行时维度
	// （type:vector 无维度限定，与 unify-vector-column-dimension 口径一致），
	// 只为已建立的事件线计算一次，不逐篇计算。
	Embedding        string `gorm:"type:vector" json:"-"`
	SubjectEmbedding string `gorm:"type:vector" json:"-"`
	// MergedInto 指向合并后的存活事件线；读取路径一跳解析（不允许两层链）。
	MergedInto *uint     `json:"merged_into,omitempty"`
	Origin     string    `gorm:"not null;default:rule" json:"origin"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (BoardStory) TableName() string {
	return "board_stories"
}

// StorySignal — 热度证据：一行 = 一个参与者（第一版 participant_key =
// "feed:<id>"）在信源时间（articles.pub_date，缺失回退 created_at）讨论过
// 一条事件线（specs/story-heat-evidence）。
//
// 身份（事件归属）与热度（signals）分离：去重只作用于身份层；本表按
// (story_id, article_id) 复合主键幂等收敛，重复写入保留最新 observed_at。
type StorySignal struct {
	// 复合主键 (story_id, article_id)：同一篇文章对同一条事件线只计一次
	// 证据；autoIncrement:false 关掉 GORM 对整型主键的默认自增。
	StoryID   uint `gorm:"primaryKey;autoIncrement:false" json:"story_id"`
	ArticleID uint `gorm:"primaryKey;autoIncrement:false" json:"article_id"`
	FeedID    uint `gorm:"not null" json:"feed_id"`
	// ParticipantKey 参与者键的 feed 兜底值（"feed:<id>"）。去重键 =
	// COALESCE(NULLIF(content_key,''), participant_key)（内容身份优先、
	// feed 兜底，fix-duplicate-article-heat D3）：content_key 为空（内容身份
	// 不可判定）时同一 feed 的多条文章只计一次。
	ParticipantKey string `gorm:"not null" json:"participant_key"`
	// ContentKey 内容身份键（规范化链接，fix-duplicate-article-heat D2）：
	// 同一篇内容被多个 feed 抓取时链接相同，去重按它而不是 feed。
	// 空串表示内容身份不可判定（link 为空），消费者退化为 participant_key。
	ContentKey string    `gorm:"column:content_key" json:"content_key"`
	Kind       string    `gorm:"not null;default:editorial" json:"kind"`
	ObservedAt time.Time `gorm:"not null" json:"observed_at"`
	// ObservedAtSource 记录 observed_at 的来源：pub_date（信源时间，优先）
	// / fallback（文章缺 pub_date 时回退 created_at）。
	ObservedAtSource string `gorm:"not null;default:pub_date" json:"observed_at_source"`
}

func (StorySignal) TableName() string {
	return "story_signals"
}
