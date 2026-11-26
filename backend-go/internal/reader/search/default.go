package search

// add-article-search tasks 2.6/2.8 — 进程级词典缓存单例。
//
// 一份 DictCache 供三方共享：/api/search 的 Service（2.6）、articles
// 列表端点搜索过滤（2.8 ListSearchWhere）、tagmanagement 标签写入方的
// InvalidateDict 失效钩子——一次失效、全消费方下次查询重载。
//
// tagmanagement → reader/search 是 change add-article-search 声明的合法
// 单向接线（depguard deny 列表不含 reader/search；本包不反向 import
// tagmanagement，无环）。

import (
	"sync"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
)

var (
	defaultDictMu sync.Mutex
	defaultDict   *DictCache
	defaultDictDB *gorm.DB
)

// DefaultDict returns the process-wide topic dictionary cache bound to the
// global database.DB. Re-binds automatically when the global DB is swapped
// （测试装配常换内存库；换库本身即失效——新缓存未加载）。nil DB 时返回
// 未绑定的缓存：EnsureLoaded 报错 → L1 静默跳过，关键词路照常（红线 11）。
func DefaultDict() *DictCache {
	defaultDictMu.Lock()
	defer defaultDictMu.Unlock()
	if defaultDict == nil || defaultDictDB != database.DB {
		defaultDictDB = database.DB
		defaultDict = NewDictCache(database.DB)
	}
	return defaultDict
}
