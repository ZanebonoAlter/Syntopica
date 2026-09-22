package imageproxy

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 上游返回 200 图片两次：第一次 MISS 落缓存，第二次 HIT 零上游。

func TestSecondRequestHitsCacheWithoutUpstream(t *testing.T) {
	srv, hits, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)
	imgURL := srv.URL + "/cover.png"

	first := doGet(t, r, imgURL, nil)
	if first.Code != http.StatusOK || first.Header().Get(cacheHeader) != "MISS" {
		t.Fatalf("first: status=%d %s=%q, want 200/MISS", first.Code, cacheHeader, first.Header().Get(cacheHeader))
	}

	second := doGet(t, r, imgURL, nil)
	if second.Code != http.StatusOK || second.Header().Get(cacheHeader) != "HIT" {
		t.Fatalf("second: status=%d %s=%q, want 200/HIT", second.Code, cacheHeader, second.Header().Get(cacheHeader))
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hits = %d, want 1（命中不再打上游）", n)
	}
	if !bytes.Equal(second.Body.Bytes(), png1x1) {
		t.Errorf("HIT 响应字节与上游图片不一致")
	}
	if ct := second.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("HIT Content-Type = %q, want image/png（嗅探）", ct)
	}
	// 缓存文件名 = sha256(原始 URL)。
	files, err := os.ReadDir(h.cache.dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("缓存文件数 = %d (err=%v), want 1", len(files), err)
	}
	if want := cacheKey(imgURL); files[0].Name() != want {
		t.Errorf("缓存文件名 = %s, want %s", files[0].Name(), want)
	}
}

// spec Scenario：非图片响应不缓存（200 text/html 透传但零落盘）。

func TestNonImage200NotCached(t *testing.T) {
	html := "<html>anti-leech</html>"
	srv, hits, _ := countingUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	})
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)
	imgURL := srv.URL + "/redirect.html"

	w := doGet(t, r, imgURL, nil)
	if w.Code != http.StatusOK || w.Body.String() != html {
		t.Fatalf("status=%d body=%q, want 200 + 原文透传", w.Code, w.Body.String())
	}
	files, err := os.ReadDir(h.cache.dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("缓存目录有 %d 个文件, want 0", len(files))
	}

	// 再请求一次：仍打上游（未缓存）。
	if w2 := doGet(t, r, imgURL, nil); w2.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200", w2.Code)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("upstream hits = %d, want 2", n)
	}
}

// spec Scenario：缓存超限滚动淘汰（旧→新删到 90% 水位，最新保留）。

func TestEvictionKeepsNewestUnderMax(t *testing.T) {
	srv, _, _ := countingUpstream(t, imageUpstream())
	size := int64(len(png1x1))
	// 上限 = 3 张图：第 4 张写入后总量 4s > 3s 触发，目标 2.7s → 删两张旧的。
	h := newTestHandler(t, 3*size, 2*time.Second)
	r := newTestEngine(t, h)

	paths := map[string]string{}
	names := []string{"a", "b", "c"}
	for _, n := range names {
		u := srv.URL + "/" + n + ".png"
		if w := doGet(t, r, u, nil); w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", n, w.Code)
		}
		paths[n] = h.cache.pathFor(u)
	}
	// 显式拉开 mtime，淘汰顺序与文件系统时间戳粒度无关。
	base := time.Now().Add(-time.Hour)
	for i, n := range names {
		if err := os.Chtimes(paths[n], base.Add(time.Duration(i)*time.Minute), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	u4 := srv.URL + "/d.png"
	if w := doGet(t, r, u4, nil); w.Code != http.StatusOK {
		t.Fatalf("d: status = %d", w.Code)
	}
	paths["d"] = h.cache.pathFor(u4)

	for _, n := range []string{"a", "b"} {
		if _, err := os.Stat(paths[n]); !os.IsNotExist(err) {
			t.Errorf("%s 应被淘汰 (err=%v)", n, err)
		}
	}
	for _, n := range []string{"c", "d"} {
		if _, err := os.Stat(paths[n]); err != nil {
			t.Errorf("%s 应保留 (err=%v)", n, err)
		}
	}
	var total int64
	entries, _ := os.ReadDir(h.cache.dir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	if total > 3*size {
		t.Errorf("总量 = %d, want ≤ %d（上限内）", total, 3*size)
	}
}

// IMAGE_CACHE_MAX_MB=0 禁用缓存：两次都打上游，均 MISS，零落盘。

func TestCacheDisabledByZeroMaxBytes(t *testing.T) {
	srv, hits, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 0, 2*time.Second)
	r := newTestEngine(t, h)
	imgURL := srv.URL + "/x.png"

	for i := 1; i <= 2; i++ {
		w := doGet(t, r, imgURL, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("req %d status = %d", i, w.Code)
		}
		if got := w.Header().Get(cacheHeader); got != "MISS" {
			t.Errorf("req %d %s = %q, want MISS", i, cacheHeader, got)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("upstream hits = %d, want 2", n)
	}
	if entries, err := os.ReadDir(h.cache.dir); err == nil && len(entries) != 0 {
		t.Fatalf("缓存目录有文件, want 空")
	}
}

// 启动清扫：超额文件被淘汰、超龄临时文件回收、新鲜临时文件保留。

func TestSweepEvictsOverBudgetAndStaleTemp(t *testing.T) {
	dir := t.TempDir()
	size := int64(len(png1x1))
	c := newDiskCache(dir, 2*size) // 上限 2 张

	writeWithMtime := func(name string, mod time.Time) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, png1x1, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-3 * time.Hour)
	writeWithMtime("old1", old)
	writeWithMtime("old2", old.Add(time.Minute))
	writeWithMtime("new1", old.Add(time.Hour))

	// 一个超龄临时文件（进程被杀残留）与一个新鲜临时文件（在写）。
	writeWithMtime(tempPrefix+"stale", old.Add(-2*time.Hour))
	writeWithMtime(tempPrefix+"fresh", time.Now())

	c.sweep()

	if _, err := os.Stat(filepath.Join(dir, "old1")); !os.IsNotExist(err) {
		t.Errorf("old1 应被启动清扫淘汰 (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, tempPrefix+"stale")); !os.IsNotExist(err) {
		t.Errorf("超龄临时文件应被回收 (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, tempPrefix+"fresh")); err != nil {
		t.Errorf("新鲜临时文件应保留 (err=%v)", err)
	}
}

// lookup 刷新 mtime（LRU touch）。

func TestLookupRefreshesMTime(t *testing.T) {
	dir := t.TempDir()
	c := newDiskCache(dir, 1<<20)
	raw := "https://cdn.example/img.png"
	p := c.pathFor(raw)
	if err := os.WriteFile(p, png1x1, 0o640); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	got, ok := c.lookup(raw)
	if !ok || got != p {
		t.Fatalf("lookup = (%q, %v), want (%q, true)", got, ok, p)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().After(old.Add(time.Hour)) {
		t.Errorf("mtime 未刷新: %v", st.ModTime())
	}
}

// IMAGE_CACHE_MAX_MB 解析矩阵。

func TestMaxBytesFromEnv(t *testing.T) {
	cases := []struct {
		val  string
		want int64
	}{
		{"", DefaultCacheMaxMB << 20},
		{"0", 0},
		{"12", 12 << 20},
		{"-1", DefaultCacheMaxMB << 20},
		{"abc", DefaultCacheMaxMB << 20},
		{" 64 ", 64 << 20},
	}
	for _, tc := range cases {
		t.Run("["+tc.val+"]", func(t *testing.T) {
			t.Setenv(CacheSizeEnv, tc.val)
			if got := maxBytesFromEnv(); got != tc.want {
				t.Errorf("maxBytesFromEnv(%q) = %d, want %d", tc.val, got, tc.want)
			}
		})
	}
}

// 缓存目录不存在时 beginWrite 自动创建。

func TestBeginWriteCreatesDir(t *testing.T) {
	c := newDiskCache(filepath.Join(t.TempDir(), "nested", "image-cache"), 1<<20)
	f, p, err := c.beginWrite()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(png1x1)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	c.commit("https://cdn.example/y.png", p)
	if _, err := os.Stat(c.pathFor("https://cdn.example/y.png")); err != nil {
		t.Fatalf("commit 后缓存文件缺失: %v", err)
	}
}
