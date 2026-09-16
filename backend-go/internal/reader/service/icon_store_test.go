package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// pngBytes is a minimal 1x1 PNG payload used by icon download test servers.
var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}

// icoBytes is a minimal ICO payload.
var icoBytes = []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10}

func TestSaveFeedIcon_DownloadsAndStoresPNG(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	store := NewIconStore(dir)
	localPath, err := store.SaveFeedIcon(42, srv.URL+"/icon.png")
	if err != nil {
		t.Fatalf("SaveFeedIcon: %v", err)
	}

	wantPath := "/icons/feeds/42.png"
	if localPath != wantPath {
		t.Errorf("localPath = %q, want %q", localPath, wantPath)
	}

	if _, err := os.Stat(filepath.Join(dir, "feeds", "42.png")); err != nil {
		t.Fatalf("stored file not found: %v", err)
	}
}

func TestSaveFeedIcon_WritesFileWithContent(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	store := NewIconStore(dir)
	if _, err := store.SaveFeedIcon(7, srv.URL+"/logo.png"); err != nil {
		t.Fatalf("SaveFeedIcon: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "feeds", "7.png"))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(data) != string(pngBytes) {
		t.Errorf("stored bytes = %d, want %d (content mismatch)", len(data), len(pngBytes))
	}
}

func TestSaveFeedIcon_RejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	store := NewIconStore(t.TempDir())
	if _, err := store.SaveFeedIcon(1, srv.URL+"/icon.png"); err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "feeds", "1.png")); !os.IsNotExist(err) {
		t.Error("no file should be written on failed download")
	}
}

func TestSaveFeedIcon_RejectsNonImageContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not an image</html>"))
	}))
	defer srv.Close()

	store := NewIconStore(t.TempDir())
	if _, err := store.SaveFeedIcon(2, srv.URL+"/icon.png"); err == nil {
		t.Fatal("expected error for text/html response, got nil")
	}
}

func TestSaveFeedIcon_RejectsOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(make([]byte, maxIconBytes+1))
	}))
	defer srv.Close()

	store := NewIconStore(t.TempDir())
	if _, err := store.SaveFeedIcon(3, srv.URL+"/big.png"); err == nil {
		t.Fatal("expected error for oversize body, got nil")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "feeds", "3.png")); !os.IsNotExist(err) {
		t.Error("no file should be written for oversize body")
	}
}

func TestSaveFeedIcon_RejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
	}))
	defer srv.Close()

	store := NewIconStore(t.TempDir())
	if _, err := store.SaveFeedIcon(4, srv.URL+"/empty.png"); err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
}

func TestSaveFeedIcon_RejectsNonHTTPScheme(t *testing.T) {
	store := NewIconStore(t.TempDir())
	if _, err := store.SaveFeedIcon(5, "file:///etc/passwd"); err == nil {
		t.Fatal("expected error for file:// URL, got nil")
	}
}

func TestSaveFeedIcon_OctetStreamRequiresIconURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(icoBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewIconStore(dir)

	// URL naming a known image extension is accepted.
	if _, err := store.SaveFeedIcon(8, srv.URL+"/favicon.ico"); err != nil {
		t.Fatalf("octet-stream with .ico URL should be accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feeds", "8.ico")); err != nil {
		t.Errorf("expected 8.ico to be written: %v", err)
	}

	// Opaque URL without a recognizable image extension is rejected.
	if _, err := store.SaveFeedIcon(9, srv.URL+"/download"); err == nil {
		t.Fatal("expected error for octet-stream with opaque URL, got nil")
	}
}

func TestSaveFeedIcon_DerivesExtFromURLForGenericImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-unknown")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	store := NewIconStore(t.TempDir())
	localPath, err := store.SaveFeedIcon(10, srv.URL+"/avatar.webp")
	if err != nil {
		t.Fatalf("SaveFeedIcon: %v", err)
	}
	if localPath != "/icons/feeds/10.webp" {
		t.Errorf("localPath = %q, want /icons/feeds/10.webp", localPath)
	}
}

func TestSaveFeedIcon_ExtensionChangeCleansOldFile(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/b.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := NewIconStore(dir)
	if _, err := store.SaveFeedIcon(11, srv.URL+"/a.png"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feeds", "11.png")); err != nil {
		t.Fatalf("11.png should exist after first save: %v", err)
	}

	if _, err := store.SaveFeedIcon(11, srv.URL+"/b.jpg"); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feeds", "11.png")); !os.IsNotExist(err) {
		t.Error("old 11.png should be removed after extension change")
	}
	if _, err := os.Stat(filepath.Join(dir, "feeds", "11.jpg")); err != nil {
		t.Errorf("new 11.jpg should exist: %v", err)
	}
}

func TestRemoveFeedIcon_DeletesAllExtensions(t *testing.T) {
	dir := t.TempDir()
	feedDir := filepath.Join(dir, "feeds")
	if err := os.MkdirAll(feedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"12.png", "12.ico"} {
		if err := os.WriteFile(filepath.Join(feedDir, f), pngBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A different feed's file must be left alone.
	if err := os.WriteFile(filepath.Join(feedDir, "13.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewIconStore(dir)
	if err := store.RemoveFeedIcon(12); err != nil {
		t.Fatalf("RemoveFeedIcon: %v", err)
	}
	for _, f := range []string{"12.png", "12.ico"} {
		if _, err := os.Stat(filepath.Join(feedDir, f)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", f)
		}
	}
	if _, err := os.Stat(filepath.Join(feedDir, "13.png")); err != nil {
		t.Errorf("other feed's icon should survive: %v", err)
	}
}

func TestRemoveFeedIcon_NoFilesIsNoError(t *testing.T) {
	store := NewIconStore(t.TempDir())
	if err := store.RemoveFeedIcon(999); err != nil {
		t.Fatalf("RemoveFeedIcon for missing files should not error: %v", err)
	}
}

// TestIconStore_PerFeedLockSerializesSameFeedWrites verifies writes to the same
// feed are serialized: while the feed's lock is held, writeFeedIcon blocks; a
// different feed's write proceeds independently.
func TestIconStore_PerFeedLockSerializesSameFeedWrites(t *testing.T) {
	store := NewIconStore(t.TempDir())

	hold := store.lockFeed(42)

	blocked := make(chan struct{})
	go func() {
		defer close(blocked)
		_, _ = store.writeFeedIcon(42, "png", pngBytes)
	}()

	select {
	case <-blocked:
		t.Fatal("writeFeedIcon must block while the same feed's lock is held")
	case <-time.After(100 * time.Millisecond):
	}

	// A different feed must not be blocked by feed 42's lock.
	other := make(chan struct{})
	go func() {
		defer close(other)
		_, _ = store.writeFeedIcon(43, "png", pngBytes)
	}()
	select {
	case <-other:
	case <-time.After(2 * time.Second):
		t.Fatal("a different feed's write must not block on feed 42's lock")
	}

	hold() // release before asserting the blocked writer proceeds
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("writeFeedIcon did not proceed after the feed lock was released")
	}
}

// TestIconStore_ConcurrentSameFeedWrites is a smoke test that many concurrent
// saves of the same feed (alternating extensions) all succeed and leave exactly
// one surviving file — the stale-extension cleanup must never orphan a returned
// path mid-write.
func TestIconStore_ConcurrentSameFeedWrites(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/b.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := NewIconStore(dir)
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, err := store.SaveFeedIcon(42, srv.URL+"/a.png")
				errs <- err
			} else {
				_, err := store.SaveFeedIcon(42, srv.URL+"/b.jpg")
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent save failed: %v", err)
		}
	}

	matches, err := filepath.Glob(filepath.Join(dir, "feeds", "42.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Errorf("expected exactly one surviving icon file, got %d: %v", len(matches), matches)
	}
}

func TestIconStorageDir_DefaultsToDataIcons(t *testing.T) {
	if got := IconStorageDir(); got == "" {
		t.Error("IconStorageDir should never be empty")
	}
	if strings.Contains(IconStorageDir(), "..") {
		t.Errorf("unexpected icon dir %q", IconStorageDir())
	}
}

// TestLocalIconExists covers the disk half of the icon state machine: a
// /icons/... path is only "present" when a regular file backs it, and malformed
// or out-of-tree paths must never be treated as present.
func TestLocalIconExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewIconStore(dir)

	localPath, err := store.SaveFeedIcon(7, srv.URL+"/icon.png")
	if err != nil {
		t.Fatalf("SaveFeedIcon: %v", err)
	}
	if localPath != "/icons/feeds/7.png" {
		t.Fatalf("localPath = %q, want /icons/feeds/7.png", localPath)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "stored file exists", path: localPath, want: true},
		{name: "file never written", path: "/icons/feeds/999.png", want: false},
		{name: "dir creation pending", path: "/icons/feeds", want: false},
		{name: "non-directory path component", path: "/icons/feeds/7.png/x", want: false},
		{name: "empty path", path: "", want: false},
		{name: "not an icons path", path: "/static/7.png", want: false},
		{name: "remote URL", path: "https://example.com/favicon.ico", want: false},
		{name: "parent traversal", path: "/icons/feeds/../../etc/passwd", want: false},
		{name: "dot segment", path: "/icons/./feeds/7.png", want: false},
		{name: "backslash separator", path: `/icons/feeds\7.png`, want: false},
		{name: "nested traversal only", path: "/icons/../icons/feeds/7.png", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := store.LocalIconExists(tt.path); got != tt.want {
				t.Errorf("LocalIconExists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	// Deleting the file (runtime dir wiped / restore without assets) flips the
	// answer to false so the refresh pipeline re-downloads instead of 404ing.
	if err := os.Remove(filepath.Join(dir, "feeds", "7.png")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if store.LocalIconExists(localPath) {
		t.Errorf("LocalIconExists(%q) = true after the file was removed, want false", localPath)
	}
}
