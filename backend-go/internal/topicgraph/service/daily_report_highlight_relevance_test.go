package service

// 「今日重点」的版块贴题档、候选池与展示序
// （rank-report-highlights-by-board-relevance 2.1 / 2.2）。
//
// 纯逻辑，无 DB：贴题档、候选池、后排三个函数只吃聚簇 + 板块信号投影。
// 夹具用「中东地缘政治与美伊关系」2026-11-27 / 2026-11-26 的真实形态（板块
// lane_tier / topic_status_at_report / cluster_tag_ids 与簇内标签、篇数照抄
// explore-findings 的回放明细）——那正是「跑题大新闻靠篇数拿走头条」的两期。

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

func relevanceSection(tagIDs []uint, laneTier, statusAtReport string) repository.ReportSectionSignal {
	return repository.ReportSectionSignal{TagIDs: tagIDs, LaneTier: laneTier, TopicStatusAtReport: statusAtReport}
}

func relevanceCluster(id uint, tagIDs []uint, articles int) repository.EventCluster {
	return repository.EventCluster{
		ClusterID: id, TagIDs: tagIDs, Label: "簇", ArticleCount: articles,
		Mode: repository.EventClusterModeVector,
	}
}

func clusterIDs(clusters []repository.EventCluster) []uint {
	out := make([]uint, 0, len(clusters))
	for _, cluster := range clusters {
		out = append(out, cluster.ClusterID)
	}
	return out
}

func pickedHighlight(id uint) repository.Highlight {
	clusterID := id
	return repository.Highlight{ClusterID: &clusterID, Title: "标题", Reason: "理由", TagIDs: []uint{id}}
}

// TestEventClusterRelevanceRank：贴题档 0–5 的档位口径 + 以完整成员标签跨板块取
// 最好一档 + 话题状态缺失不惩罚（delta spec「跨板块取最好一档」）。
func TestEventClusterRelevanceRank(t *testing.T) {
	cluster := relevanceCluster(10, []uint{10, 11}, 1)
	cases := []struct {
		name    string
		signals []repository.ReportSectionSignal
		want    int
	}{
		{"l1_direct 近档", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "l1_direct", "active")}, relevanceRankDirect},
		{"watch_keyword 手动关注", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "watch_keyword", "active")}, relevanceRankWatch},
		{"watch_sentence 手动关注", []repository.ReportSectionSignal{relevanceSection([]uint{11}, "watch_sentence", "")}, relevanceRankWatch},
		{"l2_llm 话题在跟", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "l2_llm", "active")}, relevanceRankFollowed},
		{"l2_llm 话题状态缺失不惩罚", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "l2_llm", "")}, relevanceRankFollowed},
		{"l2_llm 候选话题", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "l2_llm", "candidate")}, relevanceRankCandidateTopic},
		{"l3_new 远档", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "l3_new", "candidate")}, relevanceRankNew},
		{"板块无泳道档", []repository.ReportSectionSignal{relevanceSection([]uint{10}, "", "active")}, relevanceRankNoSection},
		{"本期正文无含该簇标签的板块", []repository.ReportSectionSignal{relevanceSection([]uint{99, 98}, "l1_direct", "active")}, relevanceRankNoSection},
		{
			"跨板块取最好一档（近档 + 远档）",
			[]repository.ReportSectionSignal{
				relevanceSection([]uint{10}, "l3_new", "candidate"),
				relevanceSection([]uint{11}, "l1_direct", "active"),
			},
			relevanceRankDirect,
		},
		{
			"跨板块取最好一档（代表标签落远档、非代表成员落手动关注）",
			[]repository.ReportSectionSignal{
				relevanceSection([]uint{10}, "l3_new", "candidate"),
				relevanceSection([]uint{11}, "watch_keyword", "active"),
			},
			relevanceRankWatch,
		},
		{
			"同簇多板块取最小档（手动关注 vs 中档在跟）",
			[]repository.ReportSectionSignal{
				relevanceSection([]uint{10}, "l2_llm", "active"),
				relevanceSection([]uint{11}, "watch_sentence", "active"),
			},
			relevanceRankWatch,
		},
		{"无任何板块信号", nil, relevanceRankNoSection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, eventClusterRelevanceRank(cluster, tc.signals))
		})
	}
}

// TestSelectEventClusterCandidatePool：候选池三条分支（贴题簇充足 / 不足补位到 3 /
// 为空维持前 8）+ 贴题簇超过 8 时按既有候选序截断，含 11-27 / 11-26 真实夹具。
func TestSelectEventClusterCandidatePool(t *testing.T) {
	// 2026-11-27「中东地缘政治与美伊关系」：三个近档板块 + 中美关税（中档 + 候选
	// 话题、当天篇数最多）。
	pool1127Clusters := []repository.EventCluster{
		relevanceCluster(304290, []uint{304290}, 6),
		relevanceCluster(304637, []uint{304637, 304968, 305381, 305386}, 3),
		relevanceCluster(304710, []uint{304710, 305378}, 2),
		relevanceCluster(305251, []uint{305251}, 1),
	}
	pool1127Signals := []repository.ReportSectionSignal{
		relevanceSection([]uint{304637, 305386, 304968, 305381}, "l1_direct", "active"),
		relevanceSection([]uint{305378, 304710}, "l1_direct", "active"),
		relevanceSection([]uint{305251}, "l1_direct", "active"),
		relevanceSection([]uint{304290}, "l2_llm", "candidate"),
	}

	// 2026-11-26：两个近档 + 一个手动关注（胡塞袭沙特推升油价）+ 两条关税簇
	// （中档候选 / 远档）+ 其它远档。
	pool1126Clusters := []repository.EventCluster{
		relevanceCluster(302619, []uint{302619}, 1),
		relevanceCluster(302640, []uint{302640}, 1),
		relevanceCluster(303446, []uint{303446}, 1),
		relevanceCluster(302178, []uint{302178, 302695, 302871, 303094, 303758}, 5),
		relevanceCluster(302082, []uint{302082}, 1),
		relevanceCluster(302119, []uint{302119}, 1),
		relevanceCluster(301867, []uint{301867}, 1),
	}
	pool1126Signals := []repository.ReportSectionSignal{
		relevanceSection([]uint{302619}, "l1_direct", "active"),
		relevanceSection([]uint{302640}, "l1_direct", "active"),
		relevanceSection([]uint{303446}, "l2_llm", "active"),
		relevanceSection([]uint{302178, 302695, 302871, 303094, 303758}, "l2_llm", "candidate"),
		relevanceSection([]uint{302082}, "l3_new", "candidate"),
		relevanceSection([]uint{302119}, "l3_new", "candidate"),
		relevanceSection([]uint{301867}, "watch_sentence", "active"),
	}

	// 贴题簇只有 2 个：补位必须按贴题档（而不是按篇数）——中档簇当天只有 1 篇，
	// 远档簇有 5 / 4 篇，补位仍是中档那条。
	shortThemeClusters := []repository.EventCluster{
		relevanceCluster(10, []uint{10}, 3),
		relevanceCluster(11, []uint{11}, 2),
		relevanceCluster(12, []uint{12}, 1),
		relevanceCluster(13, []uint{13}, 5),
		relevanceCluster(14, []uint{14}, 4),
	}
	shortThemeSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{10}, "l1_direct", "active"),
		relevanceSection([]uint{11}, "watch_keyword", "active"),
		relevanceSection([]uint{12}, "l2_llm", "active"),
		relevanceSection([]uint{13}, "l3_new", "candidate"),
		relevanceSection([]uint{14}, "l3_new", "candidate"),
	}

	// 其余聚簇贴题档相同：补位由既有候选序（篇数多的在前）破同分。
	fillTieClusters := []repository.EventCluster{
		relevanceCluster(20, []uint{20}, 8),
		relevanceCluster(21, []uint{21}, 2),
		relevanceCluster(22, []uint{22}, 5),
		relevanceCluster(23, []uint{23}, 4),
	}
	fillTieSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{22}, "l1_direct", "active"),
		relevanceSection([]uint{23}, "watch_keyword", "active"),
		relevanceSection([]uint{20}, "l2_llm", "candidate"),
		relevanceSection([]uint{21}, "l2_llm", "candidate"),
	}

	// 贴题簇一个都没有（新板子冷启动 / 全部远档）：维持既有候选序前 8。
	farClusters := make([]repository.EventCluster, 0, 10)
	for i := 1; i <= 10; i++ {
		farClusters = append(farClusters, relevanceCluster(uint(i), []uint{uint(i)}, 100-i))
	}

	// 贴题簇有 10 个、第 9/10 个标签数最多：仍按既有候选序取前 8。
	manyTheme := make([]repository.EventCluster, 0, 10)
	manyThemeSignals := make([]repository.ReportSectionSignal, 0, 10)
	for i := 1; i <= 10; i++ {
		tagIDs := []uint{uint(i)}
		if i >= 9 {
			tagIDs = []uint{uint(i), uint(i) + 100, uint(i) + 200, uint(i) + 300}
		}
		manyTheme = append(manyTheme, relevanceCluster(uint(i), tagIDs, 100-i))
		manyThemeSignals = append(manyThemeSignals, relevanceSection(tagIDs, "l1_direct", "active"))
	}

	// 贴题簇 4 个（其中 1 个按篇数只能排第 9），其余 6 个篇数更多但属中档/远档：
	// 先筛在篇数截断之前发生，所以 4 个贴题簇全部进池。
	fourThemeClusters := []repository.EventCluster{
		relevanceCluster(100, []uint{100}, 100),
		relevanceCluster(99, []uint{99}, 99),
		relevanceCluster(98, []uint{98}, 98),
		relevanceCluster(97, []uint{97}, 97),
		relevanceCluster(96, []uint{96}, 96),
		relevanceCluster(95, []uint{95}, 95),
		relevanceCluster(1, []uint{1}, 4),
		relevanceCluster(2, []uint{2}, 3),
		relevanceCluster(3, []uint{3}, 2),
		relevanceCluster(4, []uint{4}, 1),
	}
	fourThemeSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{100}, "l2_llm", "candidate"),
		relevanceSection([]uint{99}, "l2_llm", "candidate"),
		relevanceSection([]uint{98}, "l3_new", "candidate"),
		relevanceSection([]uint{97}, "l3_new", "candidate"),
		relevanceSection([]uint{96}, "l3_new", "candidate"),
		relevanceSection([]uint{95}, "l3_new", "candidate"),
		relevanceSection([]uint{1}, "l1_direct", "active"),
		relevanceSection([]uint{2}, "watch_keyword", "active"),
		relevanceSection([]uint{3}, "l1_direct", "active"),
		relevanceSection([]uint{4}, "l1_direct", "active"),
	}

	// 贴题档相同：同事件多标签大簇（17 个标签 / 20 篇）仍据篇数排池内首位。
	bigCompositeTags := make([]uint, 0, 17)
	for i := 0; i < 17; i++ {
		bigCompositeTags = append(bigCompositeTags, uint(500+i))
	}
	bigCompositeClusters := []repository.EventCluster{
		relevanceCluster(500, bigCompositeTags, 20),
		relevanceCluster(601, []uint{601}, 1),
		relevanceCluster(602, []uint{602}, 1),
		relevanceCluster(603, []uint{603}, 1),
	}
	bigCompositeSignals := []repository.ReportSectionSignal{
		relevanceSection(bigCompositeTags, "l1_direct", "active"),
		relevanceSection([]uint{601}, "l1_direct", "active"),
		relevanceSection([]uint{602}, "l1_direct", "active"),
		relevanceSection([]uint{603}, "l1_direct", "active"),
	}

	cases := []struct {
		name     string
		clusters []repository.EventCluster
		signals  []repository.ReportSectionSignal
		want     []uint
	}{
		{
			name:     "贴题簇 4 个（含篇数排第 9 的那个）全部进池，篇数更多的中桳/远档簇不进",
			clusters: fourThemeClusters,
			signals:  fourThemeSignals,
			want:     []uint{1, 2, 3, 4},
		},
		{
			name:     "贴题档相同时同事件多标签大簇排池内首位",
			clusters: bigCompositeClusters,
			signals:  bigCompositeSignals,
			want:     []uint{500, 601, 602, 603},
		},		{
			name:     "贴题簇充足：11-27 文章最多的中美关税簇不进候选池",
			clusters: pool1127Clusters,
			signals:  pool1127Signals,
			want:     []uint{304637, 304710, 305251},
		},
		{
			name:     "贴题簇充足：11-26 手动关注进池、两条关税簇不进池",
			clusters: pool1126Clusters,
			signals:  pool1126Signals,
			want:     []uint{301867, 302619, 302640},
		},
		{
			name:     "贴题簇不足：按贴题档补位到 3 且挤不掉贴题簇",
			clusters: shortThemeClusters,
			signals:  shortThemeSignals,
			want:     []uint{10, 11, 12},
		},
		{
			name:     "贴题簇不足：补位同档按既有候选序",
			clusters: fillTieClusters,
			signals:  fillTieSignals,
			want:     []uint{22, 23, 20},
		},
		{
			name: "贴题簇不足且当天再无其他聚簇：池子就是这 1-2 个",
			clusters: []repository.EventCluster{
				relevanceCluster(10, []uint{10}, 2),
				relevanceCluster(11, []uint{11}, 1),
			},
			signals: []repository.ReportSectionSignal{
				relevanceSection([]uint{10}, "l1_direct", "active"),
				relevanceSection([]uint{11}, "l2_llm", "candidate"),
			},
			want: []uint{10, 11},
		},
		{
			name:     "贴题簇为空：维持既有候选序前 8",
			clusters: farClusters,
			signals:  nil,
			want:     []uint{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:     "贴题簇超过 8 个：按既有候选序取前 8，第 9 个标签数最多也不进",
			clusters: manyTheme,
			signals:  manyThemeSignals,
			want:     []uint{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:     "零聚簇：空池（调用方走标签回退）",
			clusters: nil,
			signals:  nil,
			want:     []uint{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ordered := repository.OrderEventClusterCandidates(tc.clusters)
			pool := selectEventClusterCandidatePool(ordered, tc.signals, maxHighlightCandidatePool, minHighlightCandidatePool)
			require.Equal(t, tc.want, clusterIDs(pool))
		})
	}
}

// TestSelectEventClusterCandidatePoolKeepsCandidateOrderInsideTheme：贴题簇内部完全
// 沿用既有候选序（篇数降序），贴题档相同也不按别的键重排（design D7）。
func TestSelectEventClusterCandidatePoolKeepsCandidateOrderInsideTheme(t *testing.T) {
	clusters := []repository.EventCluster{
		relevanceCluster(30, []uint{30}, 1),
		relevanceCluster(31, []uint{31}, 7),
		relevanceCluster(32, []uint{32}, 4),
	}
	signals := []repository.ReportSectionSignal{
		relevanceSection([]uint{30}, "l1_direct", "active"),
		relevanceSection([]uint{31}, "watch_sentence", "active"),
		relevanceSection([]uint{32}, "l1_direct", "active"),
	}
	pool := selectEventClusterCandidatePool(repository.OrderEventClusterCandidates(clusters), signals, maxHighlightCandidatePool, minHighlightCandidatePool)
	require.Equal(t, []uint{31, 32, 30}, clusterIDs(pool), "贴题簇内部仍按簇内文章数降序")
}

// TestRankPickedHighlightsByRelevance：挑完再排——同一个贴题档稳定排序、同档保持
// 候选序，只排序不增删（delta spec「贴题档低的大簇让位」「手动关注命中排在近档
// 之后」「候选话题同档靠后且缺失不惩罚」「正文无对应板块的重点排最后」）。
func TestRankPickedHighlightsByRelevance(t *testing.T) {
	// 三个簇：近档 / 手动关注 / 中档候选，候选序（篇数）为大 → 中 → 小。
	pool := []repository.EventCluster{
		relevanceCluster(50, []uint{50}, 9), // 近档
		relevanceCluster(51, []uint{51}, 5), // 手动关注
		relevanceCluster(52, []uint{52}, 1), // 中档候选话题
	}
	signals := []repository.ReportSectionSignal{
		relevanceSection([]uint{50}, "l1_direct", "active"),
		relevanceSection([]uint{51}, "watch_keyword", "active"),
		relevanceSection([]uint{52}, "l2_llm", "candidate"),
	}

	noSectionPool := []repository.EventCluster{
		relevanceCluster(60, []uint{60}, 1),
		relevanceCluster(61, []uint{61}, 1),
	}
	noSectionSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{61}, "l1_direct", "active"),
		relevanceSection([]uint{99}, "l1_direct", "active"),
	}

	missingStatusPool := []repository.EventCluster{
		relevanceCluster(70, []uint{70}, 6),
		relevanceCluster(71, []uint{71}, 1),
	}
	missingStatusSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{70}, "l2_llm", "candidate"),
		relevanceSection([]uint{71}, "l2_llm", ""), // 历史快照缺失：不惩罚
	}

	// 真同档、且输入序与任何次键（簇内文章数 / 标签数 / 代表标签 id）相反：
	// x（档 0，1 篇）、y（档 5，无对应板块）、z（档 0，9 篇）——三条的篇数顺序
	// 是 z > y > x，输入序却是 x → y → z。期望 [x, z, y]：x 排在 z 前只因为它在
	// 输入（= 候选序）里在前，不是因为篇数多。
	sameRankPool := []repository.EventCluster{
		relevanceCluster(80, []uint{80}, 1), // x：档 0
		relevanceCluster(81, []uint{81}, 5), // y：档 5（无对应板块）
		relevanceCluster(82, []uint{82}, 9), // z：档 0
	}
	sameRankSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{80}, "l1_direct", "active"),
		relevanceSection([]uint{82}, "l1_direct", "active"),
	}

	// 两条同属中档、一条在跟一条状态缺失：同档不因状态不同而调换（稳定排序）。
	sameBandPool := []repository.EventCluster{
		relevanceCluster(90, []uint{90}, 4),
		relevanceCluster(91, []uint{91}, 2),
	}
	sameBandSignals := []repository.ReportSectionSignal{
		relevanceSection([]uint{90}, "l2_llm", ""),       // 缺失
		relevanceSection([]uint{91}, "l2_llm", "active"), // 在跟
	}

	nilClusterID := repository.Highlight{Title: "无聚簇身份", Reason: "理由"}

	cases := []struct {
		name    string
		picked  []repository.Highlight
		pool    []repository.EventCluster
		signals []repository.ReportSectionSignal
		want    []uint // 0 = 无 cluster_id
	}{
		{
			name:    "贴题档低的大簇让位",
			picked:  []repository.Highlight{pickedHighlight(52), pickedHighlight(50), pickedHighlight(51)},
			pool:    pool,
			signals: signals,
			want:    []uint{50, 51, 52},
		},
		{
			name:    "手动关注命中排在近档之后",
			picked:  []repository.Highlight{pickedHighlight(51), pickedHighlight(52), pickedHighlight(50)},
			pool:    pool,
			signals: signals,
			want:    []uint{50, 51, 52},
		},
		{
			name:    "候选话题同档靠后且缺失状态不惩罚",
			picked:  []repository.Highlight{pickedHighlight(70), pickedHighlight(71)},
			pool:    missingStatusPool,
			signals: missingStatusSignals,
			want:    []uint{71, 70},
		},
		{
			name:    "正文无对应板块的重点排最后",
			picked:  []repository.Highlight{pickedHighlight(60), pickedHighlight(61)},
			pool:    noSectionPool,
			signals: noSectionSignals,
			want:    []uint{61, 60},
		},
		{
			name:    "无聚簇身份的重点垫底（标签回退产物不参与贴题排序）",
			picked:  []repository.Highlight{nilClusterID, pickedHighlight(61)},
			pool:    noSectionPool,
			signals: noSectionSignals,
			want:    []uint{61, 0},
		},
		{
			name:    "同档保持候选序：两条都是档 0，靠前的入在前而不是篇数多的",
			picked:  []repository.Highlight{pickedHighlight(80), pickedHighlight(81), pickedHighlight(82)},
			pool:    sameRankPool,
			signals: sameRankSignals,
			want:    []uint{80, 82, 81},
		},
		{
			name:    "均属中档（在跟 / 缺失）时保持候选序",
			picked:  []repository.Highlight{pickedHighlight(90), pickedHighlight(91)},
			pool:    sameBandPool,
			signals: sameBandSignals,
			want:    []uint{90, 91},
		},
		{
			name:    "只有一条重点时不排序",
			picked:  []repository.Highlight{pickedHighlight(52)},
			pool:    pool,
			signals: signals,
			want:    []uint{52},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rankPickedHighlights(tc.picked, tc.pool, tc.signals)
			require.Len(t, got, len(tc.want), "只排序，不增删成员")
			ids := make([]uint, 0, len(got))
			for _, highlight := range got {
				if highlight.ClusterID == nil {
					ids = append(ids, 0)
					continue
				}
				ids = append(ids, *highlight.ClusterID)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}
