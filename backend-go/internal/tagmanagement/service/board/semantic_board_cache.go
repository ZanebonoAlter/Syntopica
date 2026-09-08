package board

import (
	"sync"
	"time"
)

const configTTL = 5 * time.Minute

type boardCache struct {
	mu sync.RWMutex

	// Board auxiliaries cache — invalidated on board composition change
	boardAuxiliaries    []BoardAuxiliaryLabel
	boardAuxiliariesSet bool

	// Board composites cache — same invalidation semantics as auxiliaries
	// (any board composition change invalidates both).
	boardComposites    []BoardCompositeLabel
	boardCompositesSet bool

	// Board embeddings cache — invalidated with auxiliaries
	boardEmbeddings    map[uint][]float64
	boardEmbeddingsSet bool

	// All active composite component sets cache（推导 tag 组合用）
	allCompositeSets    []CompositeComponentSet
	allCompositeSetsSet bool

	// AI config cache — TTL-based
	config     *SemanticBoardMatchConfig
	configTime time.Time
}

// CompositeComponentSet 是推导式 tag 组合匹配的全局输入：active 组合的
// 组件 ID 集合（add-composite-labels：tag 挂齐组件 ⇒ 视为挂该组合）。
type CompositeComponentSet struct {
	CompositeLabelID uint
	ComponentIDs     []uint
}

func (c *boardCache) GetBoardAuxiliaries() ([]BoardAuxiliaryLabel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.boardAuxiliariesSet {
		return nil, false
	}
	return c.boardAuxiliaries, true
}

func (c *boardCache) SetBoardAuxiliaries(data []BoardAuxiliaryLabel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boardAuxiliaries = data
	c.boardAuxiliariesSet = true
}

func (c *boardCache) GetBoardComposites() ([]BoardCompositeLabel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.boardCompositesSet {
		return nil, false
	}
	return c.boardComposites, true
}

func (c *boardCache) SetBoardComposites(data []BoardCompositeLabel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boardComposites = data
	c.boardCompositesSet = true
}

func (c *boardCache) GetAllCompositeSets() ([]CompositeComponentSet, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.allCompositeSetsSet {
		return nil, false
	}
	return c.allCompositeSets, true
}

func (c *boardCache) SetAllCompositeSets(data []CompositeComponentSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allCompositeSets = data
	c.allCompositeSetsSet = true
}

func (c *boardCache) GetBoardEmbeddings() (map[uint][]float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.boardEmbeddingsSet {
		return nil, false
	}
	return c.boardEmbeddings, true
}

func (c *boardCache) SetBoardEmbeddings(data map[uint][]float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boardEmbeddings = data
	c.boardEmbeddingsSet = true
}

func (c *boardCache) GetConfig() (*SemanticBoardMatchConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.config == nil || time.Since(c.configTime) >= configTTL {
		return nil, false
	}
	return c.config, true
}

func (c *boardCache) SetConfig(config SemanticBoardMatchConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = &config
	c.configTime = time.Now()
}

// packageBoardCache is the shared board cache instance used by both
// SemanticBoardMatchingService and SemanticBoardUpgradeService.
var packageBoardCache = &boardCache{}

// InvalidateMatchCache 导出版块匹配缓存失效入口（composition 挂载/移除等
// handler 直接写 board_composition 的路径调用——板级 auxiliaries/composites/
// embeddings 缓存无 TTL，不失效则匹配/回填持续用旧数据）。
func InvalidateMatchCache() {
	packageBoardCache.InvalidateBoardData()
}

func (c *boardCache) InvalidateBoardData() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boardAuxiliaries = nil
	c.boardAuxiliariesSet = false
	c.boardComposites = nil
	c.boardCompositesSet = false
	c.boardEmbeddings = nil
	c.boardEmbeddingsSet = false
	c.allCompositeSets = nil
	c.allCompositeSetsSet = false
}

// InvalidateConfig forces the next LoadConfig call to read from the database.
func (c *boardCache) InvalidateConfig() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = nil
	c.configTime = time.Time{}
}

// InvalidateMatchingConfigCache clears the cached matching config so the next
// LoadConfig call reads fresh values from the database.
func InvalidateMatchingConfigCache() {
	packageBoardCache.InvalidateConfig()
}

// InvalidateBoardCache clears the package-level board cache (board data + config).
// Tests must call this after ResetTestData truncates semantic_labels / board tables,
// since ResetTestData clears the DB but not this in-memory cache (stale entries
// would otherwise span test boundaries and break backfill/matching assertions).
func InvalidateBoardCache() {
	packageBoardCache.InvalidateBoardData()
	packageBoardCache.InvalidateConfig()
}
