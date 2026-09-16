package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"syntopica-backend/internal/models"
)

func TestBuildArticleFromEntryTracksOnlyRunnableStates(t *testing.T) {
	service := NewFeedService()
	entry := ParsedEntry{
		Title:       "Fresh News",
		Description: "desc",
		Content:     "content",
		Link:        "https://example.com/article",
		Author:      "bot",
	}

	tests := []struct {
		name                  string
		firecrawlEnabled      bool
		articleSummaryEnabled bool
		wantFirecrawlStatus   string
		wantSummaryStatus     string
	}{
		{
			name:                  "both enabled: summary incomplete, firecrawl pending",
			firecrawlEnabled:      true,
			articleSummaryEnabled: true,
			wantFirecrawlStatus:   "pending",
			wantSummaryStatus:     "incomplete",
		},
		{
			name:                  "summary only: summary pending, no firecrawl",
			firecrawlEnabled:      false,
			articleSummaryEnabled: true,
			wantFirecrawlStatus:   "completed",
			wantSummaryStatus:     "pending",
		},
		{
			name:                  "neither enabled: both default",
			firecrawlEnabled:      false,
			articleSummaryEnabled: false,
			wantFirecrawlStatus:   "completed",
			wantSummaryStatus:     "complete",
		},
		{
			name:                  "firecrawl only: summary complete, firecrawl pending",
			firecrawlEnabled:      true,
			articleSummaryEnabled: false,
			wantFirecrawlStatus:   "pending",
			wantSummaryStatus:     "complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed := models.Feed{FirecrawlEnabled: tt.firecrawlEnabled, ArticleSummaryEnabled: tt.articleSummaryEnabled}
			article := service.buildArticleFromEntry(feed, entry)
			if article.FirecrawlStatus != tt.wantFirecrawlStatus {
				t.Errorf("firecrawl status = %q, want %q", article.FirecrawlStatus, tt.wantFirecrawlStatus)
			}
			if article.SummaryStatus != tt.wantSummaryStatus {
				t.Errorf("summary status = %q, want %q", article.SummaryStatus, tt.wantSummaryStatus)
			}
		})
	}
}

// iconTestServers wires two httptest servers for icon resolution tests:
//   - icons: serves image payloads used as download candidates
//   - probe: serves homepage HTML used by the favicon probe
func iconTestServers(t *testing.T) (iconsURL, probeURL string) {
	t.Helper()

	icons := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img/rss.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/static/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(icons.Close)

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/with-link":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/static/icon.png"></head></html>`))
		case "/static/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/no-link":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>no icon link</title></head></html>`))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(icoBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(probe.Close)

	return icons.URL, probe.URL
}

// resolveTestService returns a FeedService wired to a temp icon store, so the
// pipeline tests never touch the configured data/icons directory.
func resolveTestService(t *testing.T) *FeedService {
	t.Helper()
	svc := NewFeedService()
	svc.iconStore = NewIconStore(t.TempDir())
	return svc
}

// storeIconFile writes a local icon file into the service's store so tests can
// exercise the "local /icons/ path with a file still on disk" branch.
func storeIconFile(t *testing.T, svc *FeedService, fileName string) {
	t.Helper()
	feedDir := filepath.Join(svc.iconStore.dir, "feeds")
	if err := os.MkdirAll(feedDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(feedDir, fileName), pngBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestResolveFeedIcon_CustomFrozen locks in: custom icons are never touched by
// the recompute pipeline, even when a download would succeed.
func TestResolveFeedIcon_CustomFrozen(t *testing.T) {
	iconsURL, _ := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(200, "", "custom", iconsURL+"/img/rss.png", "")
	if ok {
		t.Errorf("ok = true, want false (custom must be frozen)")
	}
	if icon != "" || source != "" {
		t.Errorf("custom icon must be untouched, got icon=%q source=%q", icon, source)
	}
}

// TestResolveFeedIcon_RSSImagePriority: parsed.Image downloads successfully, so
// it wins without ever probing the site homepage.
func TestResolveFeedIcon_RSSImagePriority(t *testing.T) {
	iconsURL, probeURL := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(201, "", "fallback", iconsURL+"/img/rss.png", probeURL+"/with-link")
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if source != "auto" {
		t.Errorf("source = %q, want auto", source)
	}
	if icon != "/icons/feeds/201.png" {
		t.Errorf("icon = %q, want /icons/feeds/201.png", icon)
	}
}

// TestResolveFeedIcon_RSSImageFailureFallsThroughToHTMLLink: the RSS image
// 404s, so the pipeline probes the homepage and downloads its <link rel=icon>.
func TestResolveFeedIcon_RSSImageFailureFallsThroughToHTMLLink(t *testing.T) {
	iconsURL, probeURL := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(202, "", "fallback", iconsURL+"/missing.png", probeURL+"/with-link")
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if source != "auto" {
		t.Errorf("source = %q, want auto", source)
	}
	if icon != "/icons/feeds/202.png" {
		t.Errorf("icon = %q, want /icons/feeds/202.png (from HTML link)", icon)
	}
}

// TestResolveFeedIcon_HTMLLinkMissingUsesGuess: no RSS image and no HTML icon
// link, so the {host}/favicon.ico guess is downloaded.
func TestResolveFeedIcon_HTMLLinkMissingUsesGuess(t *testing.T) {
	_, probeURL := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(203, "", "fallback", "", probeURL+"/no-link")
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if source != "auto" {
		t.Errorf("source = %q, want auto", source)
	}
	if icon != "/icons/feeds/203.ico" {
		t.Errorf("icon = %q, want /icons/feeds/203.ico (from favicon.ico guess)", icon)
	}
}

// TestResolveFeedIcon_AllCandidatesFailKeepsFallback: every candidate fails to
// download, so the feed stays fallback with mdi:rss.
func TestResolveFeedIcon_AllCandidatesFailKeepsFallback(t *testing.T) {
	iconsURL, _ := iconTestServers(t)
	svc := resolveTestService(t)

	// siteLink = icons server: homepage 404s and its /favicon.ico guess also
	// 404s, so all three pipeline tiers fail.
	icon, source, ok := svc.resolveFeedIcon(204, "", "fallback", iconsURL+"/missing.png", iconsURL)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if source != "fallback" {
		t.Errorf("source = %q, want fallback", source)
	}
	if icon != "mdi:rss" {
		t.Errorf("icon = %q, want mdi:rss", icon)
	}
}

// TestResolveFeedIcon_AutoLocalIconSkipsPipeline: an auto source whose icon is
// already a local /icons/ path AND whose file is still on disk must skip the
// whole pipeline (no download, no homepage probe) — a good downloaded icon
// survives transient remote failures instead of being downgraded to fallback.
func TestResolveFeedIcon_AutoLocalIconSkipsPipeline(t *testing.T) {
	// No test servers wired: if the pipeline ran at all it would hit the real
	// network and fail, returning a fallback result — the test then fails on ok.
	svc := resolveTestService(t)
	storeIconFile(t, svc, "206.png")

	icon, source, ok := svc.resolveFeedIcon(206, "/icons/feeds/206.png", "auto", "https://example.com/rss.png", "https://example.com")
	if ok {
		t.Fatalf("ok = true, want false (auto + present local icon must be frozen)")
	}
	if icon != "" || source != "" {
		t.Errorf("auto + local icon must be untouched, got icon=%q source=%q", icon, source)
	}
}

// TestResolveFeedIcon_AutoLocalIconMissingFileHeals locks in the self-healing
// path for this change: the DB claims the icon is localized but the file is
// gone (runtime dir wiped / DB restored from a dump), so the pipeline must run
// again and re-download instead of serving a permanent 404.
func TestResolveFeedIcon_AutoLocalIconMissingFileHeals(t *testing.T) {
	iconsURL, _ := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(208, "/icons/feeds/208.ico", "auto", iconsURL+"/img/rss.png", "")
	if !ok {
		t.Fatalf("ok = false, want true (missing local file must re-run the pipeline)")
	}
	if source != "auto" || icon != "/icons/feeds/208.png" {
		t.Errorf("got icon=%q source=%q, want /icons/feeds/208.png auto", icon, source)
	}
	if !svc.iconStore.LocalIconExists(icon) {
		t.Errorf("re-downloaded icon file %q not found on disk", icon)
	}
}

// TestResolveFeedIcon_AutoLocalIconMissingFileAllCandidatesFail: a dangling
// local path whose re-download fails must converge to the fallback placeholder
// (never keep pointing at the missing file).
func TestResolveFeedIcon_AutoLocalIconMissingFileAllCandidatesFail(t *testing.T) {
	iconsURL, _ := iconTestServers(t)
	svc := resolveTestService(t)

	// siteLink = icons server: homepage 404s and its /favicon.ico guess too.
	icon, source, ok := svc.resolveFeedIcon(209, "/icons/feeds/209.png", "auto", iconsURL+"/missing.png", iconsURL)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if source != "fallback" || icon != "mdi:rss" {
		t.Errorf("got icon=%q source=%q, want mdi:rss fallback", icon, source)
	}
}

// TestResolveFeedIcon_AutoLegacyRemoteURLStillRunsPipeline: auto with a
// still-remote (legacy unlocalized) icon must still run the pipeline to
// complete localization.
func TestResolveFeedIcon_AutoLegacyRemoteURLStillRunsPipeline(t *testing.T) {
	iconsURL, _ := iconTestServers(t)
	svc := resolveTestService(t)

	icon, source, ok := svc.resolveFeedIcon(207, "https://example.com/favicon.ico", "auto", iconsURL+"/img/rss.png", "")
	if !ok {
		t.Fatalf("ok = false, want true (legacy remote icon must be localized)")
	}
	if source != "auto" || icon != "/icons/feeds/207.png" {
		t.Errorf("got icon=%q source=%q, want /icons/feeds/207.png auto", icon, source)
	}
}

// TestResolveFeedIcon_AutoAndLegacySourcesRefresh: auto and empty (legacy)
// sources are recomputed like fallback.
func TestResolveFeedIcon_AutoAndLegacySourcesRefresh(t *testing.T) {
	iconsURL, _ := iconTestServers(t)

	tests := []struct {
		name          string
		currentSource string
	}{
		{name: "auto can be refreshed", currentSource: "auto"},
		{name: "empty source treated as recomputable (legacy rows)", currentSource: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := resolveTestService(t)
			icon, source, ok := svc.resolveFeedIcon(205, "", tt.currentSource, iconsURL+"/img/rss.png", "")
			if !ok {
				t.Fatalf("ok = false, want true")
			}
			if source != "auto" || icon != "/icons/feeds/205.png" {
				t.Errorf("got icon=%q source=%q, want /icons/feeds/205.png auto", icon, source)
			}
		})
	}
}
