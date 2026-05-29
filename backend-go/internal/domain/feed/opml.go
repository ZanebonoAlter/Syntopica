package feed

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
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
	errors := make([]string, 0)

	for _, categoryOutline := range opml.Body.Outlines {
		categoryName := categoryOutline.Text
		if categoryName == "" {
			categoryName = categoryOutline.Title
		}
		if categoryName == "" {
			categoryName = "Uncategorized"
		}

		var category models.Category
		err := database.DB.Where("name = ?", categoryName).First(&category).Error

		if err == gorm.ErrRecordNotFound {
			category = models.Category{
				Name:        categoryName,
				Slug:        models.GenerateSlug(categoryName),
				Icon:        "folder",
				Color:       "#6366f1",
				Description: "",
			}
			if err := database.DB.Create(&category).Error; err != nil {
				errors = append(errors, "Error creating category '"+categoryName+"': "+err.Error())
				continue
			}
			categoriesAdded++
		}

		for _, feedOutline := range categoryOutline.Outlines {
			xmlURL := feedOutline.XMLURL
			if xmlURL == "" {
				continue
			}

			title := feedOutline.Text
			if title == "" {
				title = feedOutline.Title
			}
			if title == "" {
				title = "Untitled Feed"
			}

			var existingFeed models.Feed
			err := database.DB.Where("url = ?", xmlURL).First(&existingFeed).Error
			if err == nil {
				continue
			}

			now := time.Now()
			feed := models.Feed{
				Title:       title,
				Description: "",
				URL:         xmlURL,
				CategoryID:  &category.ID,
				Icon:        "rss",
				Color:       "#8b5cf6",
				LastUpdated: &now,
			}

			if err := database.DB.Create(&feed).Error; err != nil {
				errors = append(errors, "Error importing feed '"+title+"': "+err.Error())
				continue
			}

			feedsAdded = append(feedsAdded, feed.ID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"feeds_added":      len(feedsAdded),
			"categories_added": categoriesAdded,
			"errors":           errors,
			"async_update":     true,
		},
		"message": "Imported successfully",
	})
}

func ExportOPML(c *gin.Context) {
	var categories []models.Category
	if err := database.DB.Preload("Feeds").Find(&categories).Error; err != nil {
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
	database.DB.Where("category_id IS NULL").Find(&uncategorizedFeeds)

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
