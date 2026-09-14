package handler

import (
	"context"
	"encoding/xml"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    OPMLHead `xml:"head"`
	Body    OPMLBody `xml:"body"`
}

type OPMLHead struct {
	Title       string `xml:"title"`
	DateCreated string `xml:"dateCreated"`
}

type OPMLBody struct {
	Outlines []OPMLOutline `xml:"outline"`
}

type OPMLOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Type     string        `xml:"type,attr"`
	Outlines []OPMLOutline `xml:"outline"`
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// refreshImportedFeed 异步刷新单个导入的 feed（best-effort：抓图标 + 抓文章）。
// 提取为包级变量作为测试接缝（同 service.SetSubscriptionFetcher 的做法）：
// 后台刷新会走真实网络 + 全局 repository，用例间会串扰（前一个用例的 goroutine
// 落库到下一个用例的测试库），测试必须换装无副作用实现。生产语义不变。
var refreshImportedFeed = func(feedID uint) {
	feedService := service.NewFeedService()
	now := time.Now()
	repository.Repo.DB().Model(&models.Feed{}).Where("id = ?", feedID).Updates(map[string]interface{}{
		"refresh_status":  "refreshing",
		"last_refresh_at": &now,
	})
	// best-effort async refresh（OPML 导入响应已完成，单 feed 刷新失败不阻断）
	_ = feedService.RefreshFeed(context.Background(), feedID)
}

// ImportOPML POST /api/opml/import-opml — 导入用户自有的 OPML 订阅清单。
//
// 契约边界（review High 3，用户拍板最小修复）：OPML 建源 MUST 走共享建源服务的
// NormalizeSubscriptionURL + CreateOrReuseFeed（地址规范化 → 按规范化 URL 短事务
// 查重/建源），不得再直写 models.Feed 旁路。
//
// 但本入口**刻意豁免 safefetch 网络验证**——与 feed_create_service.go 文件头
// 「规范化地址 → 有界安全抓取 → 可解析 RSS/Atom 才算过 → 去重/建源」的完整契约相比，
// 这里只走前半段：OPML 是用户从自建/已订阅源导出的清单，导入即恢复既有订阅关系，
// 不应因源站在服务端不可达（内网/自建源）而拒绝入库。因此条目地址只做格式校验
// （scheme / host / userinfo / 长度），内容验证交给导入后的后台 RefreshFeed 异步刷新。
func ImportOPML(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "No file provided",
		})
		return
	}

	if !endsWith(file.Filename, ".opml") && !endsWith(file.Filename, ".xml") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid file format. Please upload an OPML file.",
		})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer func() { _ = src.Close() }()

	var opml OPML
	if err := xml.NewDecoder(src).Decode(&opml); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Error parsing OPML file: " + err.Error(),
		})
		return
	}

	feedsAdded := make([]uint, 0)
	categoriesAdded := 0
	feedsReused := 0
	feedsSkipped := 0
	importErrors := make([]string, 0)

	// 共享建源服务：与 POST /api/feeds 同一套规范化与查重口径（不传抓取实现部分）。
	feedCreateSvc := service.NewFeedCreateService(repository.Repo.DB())

	for _, categoryOutline := range opml.Body.Outlines {
		categoryName := categoryOutline.Text
		if categoryName == "" {
			categoryName = categoryOutline.Title
		}
		if categoryName == "" {
			categoryName = "Uncategorized"
		}

		var category models.Category
		err := repository.Repo.DB().Where("name = ?", categoryName).First(&category).Error

		if err == gorm.ErrRecordNotFound {
			category = models.Category{
				Name:        categoryName,
				Slug:        models.GenerateSlug(categoryName),
				Icon:        "folder",
				Color:       "#6366f1",
				Description: "",
			}
			if err := repository.Repo.DB().Create(&category).Error; err != nil {
				importErrors = append(importErrors, "Error creating category '"+categoryName+"': "+err.Error())
				continue
			}
			categoriesAdded++
		}

		for _, feedOutline := range categoryOutline.Outlines {
			xmlURL := feedOutline.XMLURL
			if xmlURL == "" {
				// 无 xmlUrl 的 outline 是分组节点（嵌套分类），不是坏地址，不计入失败。
				continue
			}

			title := feedOutline.Text
			if title == "" {
				title = feedOutline.Title
			}
			if title == "" {
				title = "Untitled Feed"
			}

			// 单条坏地址只跳过自己，不中断整批导入。
			normalized, nerr := service.NormalizeSubscriptionURL(xmlURL)
			if nerr != nil {
				feedsSkipped++
				importErrors = append(importErrors, "Skipped feed '"+title+"': "+nerr.Error())
				continue
			}

			var (
				feed    *models.Feed
				created bool
			)
			// 事务边界照抄共享服务：查重与建源在同一短事务内完成，同地址（规范化后）
			// 重复 outline 只建一个 Feed，第二次走 reused。
			if terr := repository.Repo.DB().Transaction(func(tx *gorm.DB) error {
				f, c, txErr := feedCreateSvc.CreateOrReuseFeed(tx, normalized, service.FeedCreateOptions{
					Title:      title,
					CategoryID: &category.ID,
				})
				if txErr != nil {
					return txErr
				}
				feed, created = f, c
				return nil
			}); terr != nil {
				importErrors = append(importErrors, "Error importing feed '"+title+"': "+terr.Error())
				continue
			}

			if !created {
				feedsReused++
				continue
			}

			feedsAdded = append(feedsAdded, feed.ID)
		}
	}

	// Trigger background refresh for newly imported feeds to fetch icons
	if len(feedsAdded) > 0 {
		go func() {
			for _, feedID := range feedsAdded {
				refreshImportedFeed(feedID)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"feeds_added":      len(feedsAdded),
			"feeds_reused":     feedsReused,
			"feeds_skipped":    feedsSkipped,
			"categories_added": categoriesAdded,
			"errors":           importErrors,
			"async_update":     true,
		},
		"message": "Imported successfully",
	})
}

func ExportOPML(c *gin.Context) {
	var categories []models.Category
	if err := repository.Repo.DB().Preload("Feeds").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	opml := OPML{
		Version: "2.0",
		Head: OPMLHead{
			Title:       "RSS Feeds Export",
			DateCreated: time.Now().Format("Mon, 02 Jan 2006 15:04:05 GMT"),
		},
		Body: OPMLBody{
			Outlines: make([]OPMLOutline, 0),
		},
	}

	for _, category := range categories {
		categoryOutline := OPMLOutline{
			Text:     category.Name,
			Title:    category.Name,
			Outlines: make([]OPMLOutline, 0),
		}

		for _, feed := range category.Feeds {
			feedOutline := OPMLOutline{
				Type:   "rss",
				Text:   feed.Title,
				Title:  feed.Title,
				XMLURL: feed.URL,
			}
			categoryOutline.Outlines = append(categoryOutline.Outlines, feedOutline)
		}

		opml.Body.Outlines = append(opml.Body.Outlines, categoryOutline)
	}

	var uncategorizedFeeds []models.Feed
	repository.Repo.DB().Where("category_id IS NULL").Find(&uncategorizedFeeds)

	for _, feed := range uncategorizedFeeds {
		feedOutline := OPMLOutline{
			Type:   "rss",
			Text:   feed.Title,
			Title:  feed.Title,
			XMLURL: feed.URL,
		}
		opml.Body.Outlines = append(opml.Body.Outlines, feedOutline)
	}

	output, err := xml.MarshalIndent(opml, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=feeds.opml")
	c.Data(http.StatusOK, "text/xml", output)
}
