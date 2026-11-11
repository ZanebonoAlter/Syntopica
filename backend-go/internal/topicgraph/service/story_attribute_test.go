package service

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// Orchestration tests: no DB or AI. Global seams are restored with Cleanup;
// these tests must not run in parallel.
type storyAttributeAttach struct {
	reportID, threadID, storyID uint
	development                 bool
}

type storyAttributeRecall struct {
	boardID uint
	vector  string
	since   time.Time
	limit   int
}

type fakeStoryAttributeStore struct {
	threads       []repository.DailyReportThread
	listErr       error
	emptyAfterOne bool
	sectionTopics map[uint]uint
	tags          []models.TopicTag
	candidates    []repository.StoryCandidate
	recallErr     error
	articleIDs    map[uint][]uint
	earliest      time.Time
	attachErr     error
	panicOnList   bool

	listReports   []uint
	tagRequests   [][]uint
	trusted       map[uint]bool
	recalls       []storyAttributeRecall
	recent        []storyAttributeRecall
	articleCalls  [][]uint
	earliestCalls [][]uint
	created       []repository.BoardStory
	attached      []storyAttributeAttach
	merged        [][2]uint
	calls         []string
	// titleEmbedCalls / titleEmbedTitles / titleEmbedErr 是标题向量兜底
	// （D13 建线必须留下向量）的观测点。
	titleEmbedCalls  int
	titleEmbedTitles []string
	titleEmbedErr    error
}

var _ storyStore = (*fakeStoryAttributeStore)(nil)

func (s *fakeStoryAttributeStore) ListUnassignedThreads(reportID uint) ([]repository.DailyReportThread, error) {
	s.listReports = append(s.listReports, reportID)
	if s.panicOnList {
		panic("fake list panic")
	}
	if s.emptyAfterOne && len(s.listReports) > 1 {
		return nil, nil
	}
	return s.threads, s.listErr
}

func (s *fakeStoryAttributeStore) GetSectionTopicIDs(uint) (map[uint]uint, error) {
	return s.sectionTopics, nil
}

func (s *fakeStoryAttributeStore) GetEventTags(ids []uint) ([]models.TopicTag, error) {
	s.tagRequests = append(s.tagRequests, append([]uint(nil), ids...))
	return s.tags, nil
}

func (s *fakeStoryAttributeStore) SetTagFingerprintTrusted(id uint, trusted bool) error {
	if s.trusted == nil {
		s.trusted = make(map[uint]bool)
	}
	s.trusted[id] = trusted
	return nil
}

func (s *fakeStoryAttributeStore) RecallStoryCandidates(boardID uint, vector string, since time.Time, limit int) ([]repository.StoryCandidate, error) {
	s.recalls = append(s.recalls, storyAttributeRecall{boardID, vector, since, limit})
	return s.candidates, s.recallErr
}

func (s *fakeStoryAttributeStore) ListRecentStoryCandidates(boardID uint, since time.Time, limit int) ([]repository.StoryCandidate, error) {
	s.recent = append(s.recent, storyAttributeRecall{boardID: boardID, since: since, limit: limit})
	return s.candidates, nil
}

func (s *fakeStoryAttributeStore) GetStoryArticleIDs(ids []uint) (map[uint][]uint, error) {
	s.articleCalls = append(s.articleCalls, append([]uint(nil), ids...))
	return s.articleIDs, nil
}

func (s *fakeStoryAttributeStore) EarliestArticleAt(ids []uint) (time.Time, error) {
	s.earliestCalls = append(s.earliestCalls, append([]uint(nil), ids...))
	return s.earliest, nil
}

func (s *fakeStoryAttributeStore) CreateBoardStory(story *repository.BoardStory) error {
	s.calls = append(s.calls, "create")
	s.created = append(s.created, *story) // capture counts BEFORE attach
	story.ID = 501
	return nil
}

func (s *fakeStoryAttributeStore) AttachThreadToBoardStory(reportID, threadID, storyID uint, development bool) error {
	s.calls = append(s.calls, "attach")
	s.attached = append(s.attached, storyAttributeAttach{reportID, threadID, storyID, development})
	return s.attachErr
}

func (s *fakeStoryAttributeStore) MergeBoardStories(fromID, intoID uint) (uint, error) {
	s.calls = append(s.calls, "merge")
	s.merged = append(s.merged, [2]uint{fromID, intoID})
	return intoID, nil
}

func (s *fakeStoryAttributeStore) ResolveBoardStoryID(id uint) (uint, error) {
	return id, nil
}

func (s *fakeStoryAttributeStore) RecomputeBoardStoryCounts(uint) error {
	s.calls = append(s.calls, "recompute")
	return nil
}

func storyAttributeEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func storyAttributeDay() time.Time {
	return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
}

func storyAttributeFixture(t *testing.T) (*fakeStoryAttributeStore, *int) {
	t.Helper()
	s := &fakeStoryAttributeStore{
		threads: []repository.DailyReportThread{{
			ID: 31, ReportID: 21, SectionID: 41, Title: "甲公司发布芯片", Summary: "发布新芯片",
			TagIDs: repository.JSON(`[11]`), RelatedArticleIDs: repository.JSON(`[101,102]`), Embedding: "[1,0]",
		}},
		sectionTopics: map[uint]uint{41: 61},
		tags: []models.TopicTag{{ID: 11, Category: models.TagCategoryEvent, Metadata: models.MetadataMap{
			"event_keywords": []string{"甲公司", "芯片"},
			"event_fingerprint": map[string]any{
				"subject": "甲公司", "subject_key": "company:alpha", "action": "发布", "object": "芯片", "occurred_at": "2026-11-03",
			},
		}}},
	}
	oldStore, oldEmbed, oldFactory, oldTitleEmbed := storyStoreForTest, storySubjectEmbedder, storyAttributeClassifierFactory, storyTitleEmbedder
	storyStoreForTest = s
	embedCalls := 0
	storySubjectEmbedder = func(key string) ([]float64, error) {
		embedCalls++
		storyAttributeEqual(t, key, "company:alpha")
		return []float64{1, 0}, nil
	}
	storyTitleEmbedder = func(title string) ([]float64, error) {
		s.titleEmbedCalls++
		s.titleEmbedTitles = append(s.titleEmbedTitles, title)
		if s.titleEmbedErr != nil {
			return nil, s.titleEmbedErr
		}
		return []float64{0, 1}, nil
	}
	storyAttributeClassifierFactory = func() StoryClassifier {
		return nil // conservative degradation, never call real AI
	}
	t.Cleanup(func() {
		storyStoreForTest, storySubjectEmbedder, storyAttributeClassifierFactory, storyTitleEmbedder = oldStore, oldEmbed, oldFactory, oldTitleEmbed
	})
	return s, &embedCalls
}

func storyAttributeCandidate(id uint) repository.StoryCandidate {
	return repository.StoryCandidate{
		ID: id, Subject: "甲公司", SubjectKey: "company:alpha", Action: "发布", Object: "芯片",
		Origin: repository.StoryOriginRule, Distance: 0.1,
	}
}

func runStoryAttribute(t *testing.T) {
	t.Helper()
	if err := attributeStoriesForReport(7, 21, storyAttributeDay()); err != nil {
		t.Fatal(err)
	}
}

func assertStoryAttributeNoWrites(t *testing.T, s *fakeStoryAttributeStore) {
	t.Helper()
	if len(s.created)+len(s.attached)+len(s.merged) != 0 {
		t.Fatalf("unexpected writes: created=%v attached=%v merged=%v", s.created, s.attached, s.merged)
	}
	if s.threads[0].StoryID != nil {
		t.Fatal("thread must stay unassigned")
	}
}

func TestAttributeStoriesCompositeIntercept(t *testing.T) {
	for _, count := range []int{2, 3} {
		t.Run(fmt.Sprintf("%d_event_tags", count), func(t *testing.T) {
			s, embeds := storyAttributeFixture(t)
			s.threads[0].TagIDs = repository.JSON(`[11,12]`)
			s.tags = append(s.tags, models.TopicTag{ID: 12, Category: models.TagCategoryEvent})
			wantIDs := []uint{11, 12}
			if count == 3 {
				s.threads[0].TagIDs = repository.JSON(`[11,12,13]`)
				wantIDs = append(wantIDs, 13)
				s.tags = append(s.tags, models.TopicTag{ID: 13, Category: models.TagCategoryEvent})
			}
			runStoryAttribute(t)
			assertStoryAttributeNoWrites(t, s)
			storyAttributeEqual(t, s.tagRequests, [][]uint{wantIDs})
			storyAttributeEqual(t, len(s.recalls)+len(s.recent), 0)
			storyAttributeEqual(t, *embeds, 0)
		})
	}
}

func TestAttributeStoriesManualProtection(t *testing.T) {
	for _, assigned := range []bool{false, true} {
		name := "manual_split"
		if assigned {
			name = "manual_assignment"
		}
		t.Run(name, func(t *testing.T) {
			s, _ := storyAttributeFixture(t)
			s.threads[0].StoryManual = true
			if assigned {
				id := uint(90)
				s.threads[0].StoryID = &id
			}
			runStoryAttribute(t)
			storyAttributeEqual(t, len(s.created)+len(s.attached)+len(s.merged), 0)
			storyAttributeEqual(t, len(s.tagRequests), 0)
			if assigned {
				storyAttributeEqual(t, *s.threads[0].StoryID, uint(90))
			} else if s.threads[0].StoryID != nil {
				t.Fatal("manual split must stay NULL")
			}
		})
	}
}

func TestAttributeStoriesNotAttachableIsSkipped(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.candidates = []repository.StoryCandidate{storyAttributeCandidate(81)}
	s.attachErr = repository.ErrStoryThreadNotAttachable
	// The outer loop swallows ALL errors, so its nil return alone would
	// not prove this sentinel is handled as a benign skip.
	err := attributeOneThread(s, 7, 21, storyAttributeDay(), storyAttributeDay().AddDate(0, 0, -14), s.sectionTopics, s.threads[0])
	if err != nil {
		t.Fatalf("not-attachable must be skipped, got %v", err)
	}
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, false}})
	storyAttributeEqual(t, len(s.created), 0)
}

func TestAttributeStoriesTrustedFingerprintAttach(t *testing.T) {
	s, embeds := storyAttributeFixture(t)
	s.candidates = []repository.StoryCandidate{storyAttributeCandidate(81)}
	runStoryAttribute(t)
	storyAttributeEqual(t, s.trusted, map[uint]bool{11: true})
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, false}})
	storyAttributeEqual(t, len(s.created), 0)
	storyAttributeEqual(t, *embeds, 0)
	storyAttributeEqual(t, s.recalls, []storyAttributeRecall{{7, "[1,0]", storyAttributeDay().AddDate(0, 0, -repository.StoryRecallWindowDays), repository.StoryRecallLimit}})
	storyAttributeEqual(t, len(s.recent), 0)
}

func TestAttributeStoriesNoCandidatesCreatesStory(t *testing.T) {
	for _, occurred := range []string{"2026-11-03", "", "invalid"} {
		t.Run("occurred_"+occurred, func(t *testing.T) {
			s, embeds := storyAttributeFixture(t)
			s.tags[0].Metadata["event_fingerprint"].(map[string]any)["occurred_at"] = occurred
			s.earliest = storyAttributeDay().AddDate(0, 0, -4)
			runStoryAttribute(t)
			storyAttributeEqual(t, len(s.created), 1)
			story := s.created[0]
			wantOccurred := s.earliest
			if occurred == "2026-11-03" {
				wantOccurred = storyAttributeDay().AddDate(0, 0, -2)
				storyAttributeEqual(t, len(s.earliestCalls), 0)
			} else {
				storyAttributeEqual(t, s.earliestCalls, [][]uint{{101, 102}})
			}
			lane := uint(61)
			storyAttributeEqual(t, story, repository.BoardStory{
				BoardID: 7, Title: "甲公司发布芯片", Subject: "甲公司", SubjectKey: "company:alpha", Action: "发布", Object: "芯片",
				LaneID: &lane, Embedding: "[1,0]", SubjectEmbedding: repository.FloatsToPgVector([]float64{1, 0}), Origin: repository.StoryOriginRule,
				OccurredAt: &wantOccurred, FirstSeenDate: storyAttributeDay(), LastSeenDate: storyAttributeDay(), Status: repository.StoryStatusActive,
			}) // includes zero thread/article/participant counts before attach
			storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
			storyAttributeEqual(t, s.calls, []string{"create", "attach"})
			storyAttributeEqual(t, len(s.recent), 0) // empty vector recall must NOT fall back to listing
			storyAttributeEqual(t, *embeds, 1)       // new + empty SubjectMatch.Embedding: exactly once
		})
	}
}

// TestCreateStoryTitleEmbeddingFallback D13 召回盲区：thread 没有向量时，
// 建线用事件标题现算一次 embedding 写入 story.embedding（建线必须留下向量）。
func TestCreateStoryTitleEmbeddingFallback(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.threads[0].Embedding = ""
	runStoryAttribute(t)
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, s.titleEmbedCalls, 1)
	storyAttributeEqual(t, s.titleEmbedTitles, []string{"甲公司发布芯片"})
	storyAttributeEqual(t, s.created[0].Embedding, repository.FloatsToPgVector([]float64{0, 1}))
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
}

// TestCreateStoryTitleEmbeddingFailureDoesNotBlock D13：现算失败只记 warning、
// 不阻断建线（story 仍以 NULL 建，保持现状行为）。
func TestCreateStoryTitleEmbeddingFailureDoesNotBlock(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.threads[0].Embedding = ""
	s.titleEmbedErr = errors.New("embedding gateway unavailable")
	runStoryAttribute(t)
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, s.titleEmbedCalls, 1)
	storyAttributeEqual(t, s.created[0].Embedding, "")
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
}

// TestCreateStoryWithThreadEmbeddingSkipsTitleEmbed 已有 thread 向量时不再为
// 标题多花一次模型调用（只为真正缺向量的线补）。
func TestCreateStoryWithThreadEmbeddingSkipsTitleEmbed(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	runStoryAttribute(t)
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, s.titleEmbedCalls, 0)
	storyAttributeEqual(t, s.created[0].Embedding, "[1,0]")
}

func TestAttributeStoriesStageDevelopment(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	candidate := storyAttributeCandidate(81)
	candidate.OccurredAt = "2026-11-02" // One day before the input fingerprint's occurred_at.
	s.candidates = []repository.StoryCandidate{candidate}
	s.tags[0].Metadata["event_fingerprint"].(map[string]any)["action"] = "开售"
	runStoryAttribute(t)
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, true}})
	storyAttributeEqual(t, len(s.created), 0)
}

func TestAttributeStoriesMergeSameOccurrence(t *testing.T) {
	for _, ids := range [][]uint{{93, 81}, {81, 93}} {
		t.Run(fmt.Sprintf("order_%d_first", ids[0]), func(t *testing.T) {
			s, _ := storyAttributeFixture(t)
			for _, id := range ids {
				s.candidates = append(s.candidates, storyAttributeCandidate(id))
			}
			runStoryAttribute(t)
			storyAttributeEqual(t, s.merged, [][2]uint{{93, 81}})
			storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, false}})
			storyAttributeEqual(t, s.calls, []string{"merge", "attach"})
			storyAttributeEqual(t, len(s.created), 0)
		})
	}
}

func TestAttributeStoriesWithoutEmbeddingArticleOverlap(t *testing.T) {
	for _, overlap := range []bool{true, false} {
		name := "no_overlap_creates"
		if overlap {
			name = "overlap_attaches"
		}
		t.Run(name, func(t *testing.T) {
			s, _ := storyAttributeFixture(t)
			s.threads[0].Embedding = ""
			s.candidates = []repository.StoryCandidate{storyAttributeCandidate(81), storyAttributeCandidate(93)}
			s.articleIDs = map[uint][]uint{81: {999}, 93: {998}}
			if overlap {
				s.articleIDs[81] = []uint{102, 999}
			}
			runStoryAttribute(t)
			storyAttributeEqual(t, len(s.recalls), 0)
			storyAttributeEqual(t, s.recent, []storyAttributeRecall{{boardID: 7, since: storyAttributeDay().AddDate(0, 0, -repository.StoryRecallWindowDays), limit: repository.StoryRecallLimit}})
			storyAttributeEqual(t, s.articleCalls, [][]uint{{81, 93}})
			if overlap {
				storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, false}})
				storyAttributeEqual(t, len(s.created), 0)
			} else {
				storyAttributeEqual(t, len(s.created), 1)
				// D13：thread 无向量时建线用标题现算补一条向量（不再留 NULL）。
				storyAttributeEqual(t, s.titleEmbedCalls, 1)
				storyAttributeEqual(t, s.created[0].Embedding, repository.FloatsToPgVector([]float64{0, 1}))
				storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
			}
		})
	}
}

func TestAttributeStoriesRecallFailureKeepsUnassigned(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.recallErr = errors.New("recall unavailable")
	runStoryAttribute(t)
	assertStoryAttributeNoWrites(t, s)
	storyAttributeEqual(t, len(s.recent), 0)
	// A failed thread remains retryable.
	s.recallErr = nil
	runStoryAttribute(t)
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, len(s.attached), 1)
}

func TestAttributeStoriesSubjectEmbeddingReused(t *testing.T) {
	s, embeds := storyAttributeFixture(t)
	other := storyAttributeCandidate(81)
	other.Subject, other.SubjectKey, other.SubjectEmbedding = "乙公司", "company:beta", []float64{0, 1}
	s.candidates = []repository.StoryCandidate{other}
	runStoryAttribute(t)
	storyAttributeEqual(t, *embeds, 1) // Resolve computed it; create must reuse it
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, s.created[0].SubjectEmbedding, repository.FloatsToPgVector([]float64{1, 0}))
}

func TestAttributeStoriesExactSubjectNewObjectDoesNotEmbed(t *testing.T) {
	s, embeds := storyAttributeFixture(t)
	other := storyAttributeCandidate(81)
	other.Object = "手机"
	s.candidates = []repository.StoryCandidate{other}
	runStoryAttribute(t)
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, *embeds, 0)
	storyAttributeEqual(t, s.created[0].SubjectEmbedding, "")
}

func TestAttributeStoriesRepeatedRunIsIdempotent(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.emptyAfterOne = true
	runStoryAttribute(t)
	runStoryAttribute(t)
	storyAttributeEqual(t, s.listReports, []uint{21, 21})
	storyAttributeEqual(t, len(s.created), 1)
	storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
	storyAttributeEqual(t, len(s.recalls), 1)
}

func TestAttributeStoriesSpawnHook(t *testing.T) {
	calls := 0
	restore := SetAttributeStoriesSpawnForTest(func(boardID, reportID uint, date time.Time) {
		calls++
		storyAttributeEqual(t, boardID, uint(7))
		storyAttributeEqual(t, reportID, uint(21))
		storyAttributeEqual(t, date, storyAttributeDay())
	})
	t.Cleanup(restore)
	SpawnAttributeStoriesForReport(7, 21, storyAttributeDay())
	storyAttributeEqual(t, calls, 1)
}

func TestAttributeStoriesSpawnPanicRecovered(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.panicOnList = true
	restore := SetAttributeStoriesSpawnForTest(attributeStoriesSafe)
	t.Cleanup(restore)
	SpawnAttributeStoriesForReport(7, 21, storyAttributeDay())
	storyAttributeEqual(t, s.listReports, []uint{21})
	assertStoryAttributeNoWrites(t, s)
}

func TestAttributeStoriesListFailureReturnsError(t *testing.T) {
	s, _ := storyAttributeFixture(t)
	s.listErr = errors.New("list unavailable")
	if err := attributeStoriesForReport(7, 21, storyAttributeDay()); !errors.Is(err, s.listErr) {
		t.Fatalf("got %v, want wrapped list error", err)
	}
	assertStoryAttributeNoWrites(t, s)
}

func TestStoryRecallEvidenceReachesClassifierPrompt(t *testing.T) {
	for _, embedding := range []string{"[1,2]", ""} {
		t.Run("embedding="+embedding, func(t *testing.T) {
			store := &fakeStoryAttributeStore{
				candidates: []repository.StoryCandidate{{ID: 7, Title: "代表线程标题", Summary: "代表线程摘要"}},
				articleIDs: map[uint][]uint{7: {1}},
			}
			candidates, _, err := recallCandidates(store, 1, repository.DailyReportThread{Embedding: embedding}, time.Now(), []uint{1})
			if err != nil {
				t.Fatal(err)
			}
			prompt := buildStoryClassifyPrompt(AssignmentInput{}, candidates)
			if !strings.Contains(prompt, "代表线程标题") || !strings.Contains(prompt, "代表线程摘要") {
				t.Fatalf("representative evidence missing from prompt: %s", prompt)
			}
		})
	}
}

func TestAttributeStoriesStagePeriodDateFallback(t *testing.T) {
	for _, gap := range []int{1, 2} {
		for _, withEmbedding := range []bool{true, false} {
			t.Run(fmt.Sprintf("gap_%d_embedding_%t", gap, withEmbedding), func(t *testing.T) {
				s, _ := storyAttributeFixture(t)
				fp := s.tags[0].Metadata["event_fingerprint"].(map[string]any)
				fp["action"] = "开售"
				fp["occurred_at"] = ""
				c := storyAttributeCandidate(81)
				c.OccurredAt = ""
				c.PeriodDate = storyAttributeDay().AddDate(0, 0, -gap)
				s.candidates = []repository.StoryCandidate{c}
				if !withEmbedding {
					s.threads[0].Embedding = ""
					s.articleIDs = map[uint][]uint{81: {101}}
				}
				runStoryAttribute(t)
				if gap == 1 {
					storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 81, true}})
					storyAttributeEqual(t, len(s.created), 0)
				} else {
					storyAttributeEqual(t, len(s.created), 1)
					storyAttributeEqual(t, s.attached, []storyAttributeAttach{{21, 31, 501, false}})
				}
			})
		}
	}
}
