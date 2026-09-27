package service

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/logging"
)

const (
	// maxIconBytes caps how large a downloaded feed icon may be (spec: ≤256KB).
	maxIconBytes = 256 * 1024
	// iconDownloadTimeout bounds a single icon download (spec: ≤10s).
	iconDownloadTimeout = 10 * time.Second
	// defaultIconDir is the fallback storage root when storage.icon_dir is unset.
	defaultIconDir = "data/icons"
)

var (
	defaultIconStoreOnce sync.Once
	defaultIconStore     *IconStore
)

// IconStorageDir returns the configured icon storage root (viper
// storage.icon_dir, default data/icons).
func IconStorageDir() string {
	if config.AppConfig != nil && config.AppConfig.Storage.IconDir != "" {
		return config.AppConfig.Storage.IconDir
	}
	return defaultIconDir
}

// DefaultIconStore returns the process-wide icon store rooted at the configured
// storage directory, creating it lazily on first use.
func DefaultIconStore() *IconStore {
	defaultIconStoreOnce.Do(func() {
		defaultIconStore = NewIconStore(IconStorageDir())
	})
	return defaultIconStore
}

// IconStore downloads feed icons to the local filesystem so the frontend only
// ever loads same-origin addresses (served via the /icons static route).
type IconStore struct {
	dir    string       // storage root (default data/icons), holds the feeds/ subdirectory
	client *http.Client // download client
	// feedLocks serializes icon writes per feed (feedID -> *sync.Mutex) so
	// concurrent refreshes of the same feed can't have one writer's stale-file
	// cleanup delete another writer's freshly renamed file.
	feedLocks sync.Map
}

// NewIconStore creates an icon store rooted at dir. The storage directory is
// created lazily — at startup by the router (see SetupRoutes) and on every
// write — so test processes never litter the worktree with data/ directories.
func NewIconStore(dir string) *IconStore {
	return &IconStore{
		dir:    dir,
		client: httpclient.New(httpclient.WithTimeout(iconDownloadTimeout)),
	}
}

// SaveFeedIcon downloads remoteURL and writes it to feeds/<feedID>.<ext>,
// returning the local URL path (/icons/feeds/<feedID>.<ext>). The download is
// validated: http(s) URL, HTTP 200, image Content-Type, ≤256KB. The write is
// atomic (temp file + rename) and any previously stored icon for the same feed
// with a different extension is removed.
func (s *IconStore) SaveFeedIcon(feedID uint, remoteURL string) (string, error) {
	if !isSupportedIconURL(remoteURL) {
		return "", fmt.Errorf("unsupported icon URL %q", remoteURL)
	}

	resp, err := s.client.Get(remoteURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("icon download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxIconBytes {
		return "", fmt.Errorf("icon exceeds %d bytes", maxIconBytes)
	}

	ext, err := iconExtension(resp.Header.Get("Content-Type"), remoteURL)
	if err != nil {
		return "", err
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read icon body: %w", err)
	}
	if len(data) > maxIconBytes {
		return "", fmt.Errorf("icon exceeds %d bytes", maxIconBytes)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("icon body is empty")
	}

	return s.writeFeedIcon(feedID, ext, data)
}

// lockFeed returns a function releasing the per-feed write lock for feedID,
// creating the mutex on first use. Distinct feeds never block each other.
func (s *IconStore) lockFeed(feedID uint) func() {
	m, _ := s.feedLocks.LoadOrStore(feedID, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// writeFeedIcon atomically writes feed icon bytes and returns its local URL path.
func (s *IconStore) writeFeedIcon(feedID uint, ext string, data []byte) (string, error) {
	unlock := s.lockFeed(feedID)
	defer unlock()

	feedDir := filepath.Join(s.dir, "feeds")
	if err := os.MkdirAll(feedDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create icon dir: %w", err)
	}

	fileName := fmt.Sprintf("%d.%s", feedID, ext)
	finalPath := filepath.Join(feedDir, fileName)

	tmp, err := os.CreateTemp(feedDir, fmt.Sprintf(".%d-*.tmp", feedID))
	if err != nil {
		return "", fmt.Errorf("failed to create temp icon file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("failed to write icon: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp icon file: %w", err)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", fmt.Errorf("failed to move icon into place: %w", err)
	}

	s.removeStaleFeedIcons(feedID, finalPath)

	return "/icons/feeds/" + fileName, nil
}

// removeStaleFeedIcons deletes icon files belonging to the feed that are not
// the freshly written one (extension-change cleanup, glob feeds/<id>.*).
func (s *IconStore) removeStaleFeedIcons(feedID uint, keepPath string) {
	matches, err := filepath.Glob(filepath.Join(s.dir, "feeds", fmt.Sprintf("%d.*", feedID)))
	if err != nil {
		return
	}
	for _, m := range matches {
		if m != keepPath {
			_ = os.Remove(m)
		}
	}
}

// LocalIconExists reports whether a DB-stored local icon path (the
// "/icons/..." form written by writeFeedIcon) still has a backing file in this
// store. It is the disk half of the icon state machine: the DB path alone is
// not proof the file survived (dump-restored DBs, wiped runtime dirs, container
// volume loss), and a missing file must trigger a re-download rather than a
// permanent 404.
//
// "Missing" means only the errors that definitively prove the path cannot
// exist: ENOENT and ENOTDIR (a path component is a non-directory). Any other
// error (permissions, I/O) is treated as present so a transient problem never
// clobbers a good icon. Malformed or out-of-tree paths return false.
func (s *IconStore) LocalIconExists(localPath string) bool {
	rel, ok := iconRelPath(localPath)
	if !ok {
		return false
	}
	info, err := os.Stat(filepath.Join(s.dir, rel))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return false
		}
		return true
	}
	return !info.IsDir()
}

// iconRelPath converts a "/icons/..." URL path into a store-relative
// filesystem path, rejecting anything that could escape the store root
// (absolute paths, backslash separators, empty/dot/dot-dot segments).
func iconRelPath(localPath string) (string, bool) {
	const prefix = "/icons/"
	if !strings.HasPrefix(localPath, prefix) {
		return "", false
	}
	rel := strings.TrimPrefix(localPath, prefix)
	if rel == "" || strings.Contains(rel, "\\") {
		return "", false
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}
	return filepath.FromSlash(rel), true
}

// RemoveFeedIcon deletes all locally stored icon files for the feed
// (glob feeds/<id>.*). Missing files are not an error.
func (s *IconStore) RemoveFeedIcon(feedID uint) error {
	unlock := s.lockFeed(feedID)
	defer unlock()

	matches, err := filepath.Glob(filepath.Join(s.dir, "feeds", fmt.Sprintf("%d.*", feedID)))
	if err != nil {
		return fmt.Errorf("failed to enumerate feed icons: %w", err)
	}
	var firstErr error
	for _, m := range matches {
		if err := os.Remove(m); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DeleteFeedIconFiles removes any locally stored icon files for the feed.
// Failures are logged and never propagated — feed deletion must proceed.
func DeleteFeedIconFiles(feedID uint) {
	if err := DefaultIconStore().RemoveFeedIcon(feedID); err != nil {
		logging.Errorf("Failed to remove icon files for feed %d: %v", feedID, err)
	}
}

// isSupportedIconURL restricts icon downloads to http(s) URLs so stored paths
// never contain user-controlled filesystem input.
func isSupportedIconURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// iconExtension maps a download's Content-Type (with URL fallback) to a file
// extension. Accepts image/* plus the common .ico carriers
// (image/x-icon, image/vnd.microsoft.icon, application/octet-stream).
func iconExtension(contentType, remoteURL string) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "image/png":
		return "png", nil
	case "image/jpeg":
		return "jpg", nil
	case "image/gif":
		return "gif", nil
	case "image/webp":
		return "webp", nil
	case "image/svg+xml":
		return "svg", nil
	case "image/avif":
		return "avif", nil
	case "image/x-icon", "image/vnd.microsoft.icon":
		return "ico", nil
	case "application/octet-stream":
		// opaque bytes: only trust the URL when it names an image extension
		if ext := extFromIconURL(remoteURL); ext != "" {
			return ext, nil
		}
		return "", fmt.Errorf("cannot infer icon type from opaque content type and URL")
	}
	if strings.HasPrefix(ct, "image/") {
		if ext := extFromIconURL(remoteURL); ext != "" {
			return ext, nil
		}
		return "", fmt.Errorf("unsupported image content type %q without URL extension", ct)
	}
	return "", fmt.Errorf("content type %q is not an image", ct)
}

var iconURLSuffixes = []struct{ suffix, ext string }{
	{".ico", "ico"}, {".png", "png"}, {".jpg", "jpg"}, {".jpeg", "jpg"},
	{".gif", "gif"}, {".svg", "svg"}, {".webp", "webp"}, {".avif", "avif"},
}

// extFromIconURL returns a known image extension derived from the URL path
// (e.g. /favicon.ico -> ico), or "".
func extFromIconURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.ToLower(u.Path)
	for _, s := range iconURLSuffixes {
		if strings.HasSuffix(path, s.suffix) {
			return s.ext
		}
	}
	return ""
}
