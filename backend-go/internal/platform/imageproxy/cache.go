// Package imageproxy 外链图片代理（openspec change add-image-proxy）：
// 对外链图片注入防盗链所需 Referer/UA 后转发，配磁盘缓存与总量上限。
// design.md D5：仅接受 http/https 且拒绝指向代理自身（防循环），不封内网
// ——单用户本地应用，safefetch 的私网拒绝契约（discovery 场景）不适用此处。
package imageproxy

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCacheMaxMB 是磁盘缓存总量默认上限（IMAGE_CACHE_MAX_MB 可覆盖）。
	DefaultCacheMaxMB = 256
	// CacheSizeEnv 配置缓存总量上限（MB）；显式 0 禁用缓存，负数/非法回落默认。
	CacheSizeEnv = "IMAGE_CACHE_MAX_MB"

	defaultCacheDir = "data/image-cache"
	tempPrefix      = ".dl-"
	// 残留临时文件（进程中途被杀）超过该时长才允许清扫，避免误删在写的临时文件。
	staleTempAge = time.Hour
	// 淘汰水位：删到总量 ≤ 上限的 90%，留缓冲避免每次写入都全目录扫描。
	evictWatermarkNum, evictWatermarkDen = 9, 10
)

// diskCache 是磁盘内容寻址缓存：key = sha256(原始 URL)，命中以 mtime 当最近
// 访问时间（ext4 relatime 下 atime 不可靠）。maxBytes ≤ 0 表示禁用。
type diskCache struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex // 串行化落盘与淘汰
}

func newDiskCache(dir string, maxBytes int64) *diskCache {
	return &diskCache{dir: dir, maxBytes: maxBytes}
}

// maxBytesFromEnv 解析 IMAGE_CACHE_MAX_MB → 字节上限。
func maxBytesFromEnv() int64 {
	v := strings.TrimSpace(os.Getenv(CacheSizeEnv))
	if v == "" {
		return DefaultCacheMaxMB << 20
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return DefaultCacheMaxMB << 20
	}
	return n << 20
}

func cacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}

func (c *diskCache) enabled() bool { return c != nil && c.maxBytes > 0 }

func (c *diskCache) pathFor(rawURL string) string {
	return filepath.Join(c.dir, cacheKey(rawURL))
}

// lookup 返回命中文件路径并刷新 mtime；未命中或禁用返回 false。
func (c *diskCache) lookup(rawURL string) (string, bool) {
	if !c.enabled() {
		return "", false
	}
	p := c.pathFor(rawURL)
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return "", false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return p, true
}

// beginWrite 在缓存目录内开临时文件（与最终路径同文件系统，rename 才是原子的）。
// 缓存目录不可写时返回错误——调用方降级为不缓存直通，响应不受影响。
func (c *diskCache) beginWrite() (*os.File, string, error) {
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return nil, "", err
	}
	f, err := os.CreateTemp(c.dir, tempPrefix+"*")
	if err != nil {
		return nil, "", err
	}
	return f, f.Name(), nil
}

// commit 把写完的临时文件原子落为缓存并按需淘汰；rename 失败只清理临时文件。
// protected 是刚写入的文件：淘汰不得删除它（spec「最新访问的文件保留」）。
func (c *diskCache) commit(rawURL, tmpPath string) {
	final := c.pathFor(rawURL)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	c.evictLocked(final)
}

// sweep 是启动时的一次性清理：淘汰超额文件并回收陈旧临时文件。
func (c *diskCache) sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanStaleTempLocked()
	c.evictLocked("")
}

// evictLocked 总量超上限时按 mtime 从旧删到 ≤90% 水位；protected 永不删，
// 其余删完仍超水位则停止（单文件超大时允许残留它自己）。
func (c *diskCache) evictLocked(protected string) {
	if !c.enabled() {
		return
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var files []entry
	var total int64
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), tempPrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{path: filepath.Join(c.dir, e.Name()), size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return
	}
	target := c.maxBytes * evictWatermarkNum / evictWatermarkDen
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= target {
			break
		}
		if f.path == protected {
			continue
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

// cleanStaleTempLocked 回收进程中途被杀留下的超龄临时文件。
func (c *diskCache) cleanStaleTempLocked() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleTempAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), tempPrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(c.dir, e.Name()))
	}
}

// detectContentType 嗅探缓存命中的图片类型（无 meta 文件的代价，见 design D2）。
func detectContentType(head []byte) string {
	ct := http.DetectContentType(head)
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ct
}
