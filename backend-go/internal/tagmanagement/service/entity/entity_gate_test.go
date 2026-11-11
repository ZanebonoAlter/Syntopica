package entity

// 场景锚点（tasks.md 映射表）：
//   - 跨来源并发在闸内重读最新候选再二判
//   - 取消超时异常释放闸门与连接
// 白盒 B3：TestEntityGate_ReReadsAfterAcquire
//       B4：TestEntityGate_Concurrent_BothBranches
//       B5：TestEntityGate_ReleaseOnCancelOrTimeout

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repository "syntopica-backend/internal/tagmanagement/repository"
)

// holdGateManually claims the gate on a dedicated raw connection (simulating
// another process). Returns a release func.
func holdGateManually(t *testing.T, repo *repository.EntityRepository) func() {
	t.Helper()
	sqlDB, err := repo.DB().DB()
	require.NoError(t, err)
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", repository.EntityResolveGateKey); err != nil {
		_ = conn.Close()
		t.Fatalf("manual gate hold failed: %v", err)
	}
	return func() {
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", repository.EntityResolveGateKey)
		_ = conn.Close()
	}
}

// 场景 + 白盒 B3：闸内重读最新候选再二判，不提交闸外旧 none。
// 后取得闸门的一方在闸内重读候选池并按最新候选重新判定：新判定选中已登记
// 实体 → 两条提及指向同一实体 ID；闸门不被宣称为「实体全局不重」的唯一
// 机制（重复由评测量化）。
func TestEntityGate_ReReadsAfterAcquire(t *testing.T) {
	embed := &fakeEmbed{byText: map[string][]float64{}}
	noneChat := &fakeChat{responses: []string{"none"}}
	repo, db := setupEntityDB(t)
	rsFirst := NewResolver(repo, noneChat.chat, embed.embed, fastConfig())

	// 进程 A：空池 → none → 原子新建 E1。
	tagA := uint(5001)
	a := rsFirst.ResolveMention(context.Background(), MentionRequest{
		MentionText: "特朗普", SubjectDesc: "美国第45任总统", SourceTagID: &tagA,
	})
	require.Equal(t, StatusCreated, a.Status)

	// 进程 B（不同来源）：闸内重读必然看到 A 刚登记的实体（registered-name
	// 命中 + 同空间名向量召回），二轮选中 → 同一统一 ID。
	// 若实现提交闸外旧 none（无重读），B 会另建 E2 —— 实体数变 2 即红。
	smartChat := &fakeChat{pickFirst: true}
	rsB := NewResolver(repo, smartChat.chat, embed.embed, fastConfig())
	tagB := uint(5002)
	b := rsB.ResolveMention(context.Background(), MentionRequest{
		MentionText: "特朗普", SubjectDesc: "美国第45任总统", SourceTagID: &tagB,
	})
	require.Equal(t, StatusReused, b.Status, "闸内重读后按最新候选选中已登记实体")
	assert.Equal(t, a.EntityID, b.EntityID, "两条提及指向同一实体 ID")

	var entities int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry").Scan(&entities).Error)
	assert.EqualValues(t, 1, entities, "不提交闸外旧 none → 不产生重复实体")

	// 两条提及证据都挂在同一实体下（跨来源各自的行）。
	var mentions int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_mentions WHERE entity_id = ?", a.EntityID).Scan(&mentions).Error)
	assert.EqualValues(t, 2, mentions)
}

// 白盒 B4：并发两分支 —— 选中已有 ID → 同 ID；判 none → 允许新建（不算违约）。
// 闸门只防陈旧结论提交，不保证模型判别正确、不承诺实体全局不重。
func TestEntityGate_Concurrent_BothBranches(t *testing.T) {
	t.Run("branch pick-existing converges to one entity", func(t *testing.T) {
		embed := &fakeEmbed{byText: map[string][]float64{}}
		repo, db := setupEntityDB(t)
		// 每个并发 resolver 各自的 chat（pickFirst：选中池中候选）。
		const n = 4
		resolvers := make([]*Resolver, n)
		for i := range resolvers {
			c := &fakeChat{pickFirst: true}
			resolvers[i] = NewResolver(repo, c.chat, embed.embed, fastConfig())
		}
		var wg sync.WaitGroup
		results := make([]*Resolution, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				tagID := uint(6000 + i)
				results[i] = resolvers[i].ResolveMention(context.Background(), MentionRequest{
					MentionText: "英伟达", SourceTagID: &tagID,
				})
			}(i)
		}
		wg.Wait()

		for i, r := range results {
			require.NotNil(t, r, "resolver %d", i)
			assert.NotEqual(t, StatusUnresolved, r.Status, "并发不应产生技术失败：%s", r.Reason)
		}
		// 后进闸者重读后选中先建实体 → 全部收敛到一个实体 ID。
		var entities int64
		require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry WHERE canonical_name = '英伟达'").Scan(&entities).Error)
		assert.EqualValues(t, 1, entities, "选中分支：并发收敛到一个实体")
	})

	t.Run("branch legal-none allows creation, not a violation", func(t *testing.T) {
		embed := &fakeEmbed{byText: map[string][]float64{}}
		repo, db := setupEntityDB(t)
		const n = 2
		resolvers := make([]*Resolver, n)
		for i := range resolvers {
			c := &fakeChat{responses: []string{"none"}} // 恒判 none（模型判别是效果问题）
			resolvers[i] = NewResolver(repo, c.chat, embed.embed, fastConfig())
		}
		var wg sync.WaitGroup
		results := make([]*Resolution, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				tagID := uint(6100 + i)
				results[i] = resolvers[i].ResolveMention(context.Background(), MentionRequest{
					MentionText: "OpenAI", SourceTagID: &tagID,
				})
			}(i)
		}
		wg.Wait()
		for i, r := range results {
			require.NotNil(t, r, "resolver %d", i)
			assert.Equal(t, StatusCreated, r.Status, "合法 none → 新建，不算违约（%s）", r.Reason)
		}
		var entities int64
		require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry WHERE canonical_name = 'OpenAI'").Scan(&entities).Error)
		assert.EqualValues(t, 2, entities, "重复由重复身份组率与覆盖率量化——锁不保证不重")

		// 闸门未遗留占用：后续仍能正常取闸解析。
		c := &fakeChat{responses: []string{"none"}}
		rs := NewResolver(repo, c.chat, embed.embed, fastConfig())
		tagID := uint(6199)
		res := rs.ResolveMention(context.Background(), MentionRequest{MentionText: "Anthropic", SourceTagID: &tagID})
		require.Equal(t, StatusCreated, res.Status)
	})
}

// 场景 + 白盒 B5：取消超时异常释放闸门与连接。
// WHEN 持有/等待闸门的进程被取消、发生异常或等待超时：闸门被释放且占用
// 连接归还连接池，后续进程能正常取得；该次归一以明确技术失败结束，不新建
// 实体、不留半实体。
func TestEntityGate_ReleaseOnCancelOrTimeout(t *testing.T) {
	embed := &fakeEmbed{byText: map[string][]float64{}}
	noneChat := &fakeChat{responses: []string{"none"}}
	repo, db := setupEntityDB(t)

	cfg := fastConfig()
	cfg.GateWaitTimeout = 300 * time.Millisecond
	rs := NewResolver(repo, noneChat.chat, embed.embed, cfg)

	// 分支①：闸被他人持有 → 等待超时 → 明确技术失败。
	release := holdGateManually(t, repo)
	tagID := uint(7001)
	start := time.Now()
	res := rs.ResolveMention(context.Background(), MentionRequest{MentionText: "特斯拉", SourceTagID: &tagID})
	require.NotNil(t, res)
	assert.Equal(t, StatusUnresolved, res.Status)
	assert.Contains(t, res.Reason, "gate")
	assert.Contains(t, res.Reason, "timed out")
	assert.Less(t, time.Since(start), 5*time.Second, "有界等待，不无限阻塞")
	// 不新建、不半写。
	var entities int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry").Scan(&entities).Error)
	assert.EqualValues(t, 0, entities)
	release()

	// 分支②：超时路径归还了连接与闸门——持有者释放后本进程能立即取得并
	// 正常完成解析（若超时路径泄漏了带锁连接，这次取闸会永久阻塞）。
	res2 := rs.ResolveMention(context.Background(), MentionRequest{MentionText: "特斯拉", SourceTagID: &tagID})
	require.Equal(t, StatusCreated, res2.Status, "闸释放后可正常取得并完成解析")

	// 分支③：等待闸门期间 ctx 被取消 → 技术失败结束，连接归还。
	release2 := holdGateManually(t, repo)
	cancelCfg := fastConfig()
	cancelCfg.GateWaitTimeout = 10 * time.Second
	rsCancel := NewResolver(repo, noneChat.chat, embed.embed, cancelCfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *Resolution, 1)
	go func() {
		tagID2 := uint(7002)
		done <- rsCancel.ResolveMention(ctx, MentionRequest{MentionText: "SpaceX", SourceTagID: &tagID2})
	}()
	time.Sleep(200 * time.Millisecond) // 让它进入闸门等待
	cancel()
	res3 := <-done
	require.NotNil(t, res3)
	assert.Equal(t, StatusUnresolved, res3.Status, "取消 → 明确技术失败")
	release2()

	// 连接池未被耗尽：常规查询照常。
	var cnt int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry").Scan(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
}
