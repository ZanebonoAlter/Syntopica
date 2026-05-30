package tagging

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// EmbeddingConfigHandler handles HTTP requests for embedding configuration
type EmbeddingConfigHandler struct {
	configService *EmbeddingConfigService
}

// NewEmbeddingConfigHandler creates a new handler
func NewEmbeddingConfigHandler() *EmbeddingConfigHandler {
	return &EmbeddingConfigHandler{
		configService: NewEmbeddingConfigService(),
	}
}

// GetEmbeddingConfig returns all embedding config items
func GetEmbeddingConfig(c *gin.Context) {
	service := NewEmbeddingConfigService()
	configs, err := service.GetAllConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": configs})
}

// UpdateEmbeddingConfig updates a single config value
func UpdateEmbeddingConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "config key is required"})
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body, expected {\"value\": \"...\"}"})
		return
	}

	service := NewEmbeddingConfigService()
	if err := service.UpdateConfig(key, body.Value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "config updated"})
}

// RegisterEmbeddingConfigRoutes registers the embedding config API routes
func RegisterEmbeddingConfigRoutes(rg *gin.RouterGroup) {
	embGroup := rg.Group("/embedding")
	{
		embGroup.GET("/config", GetEmbeddingConfig)
		embGroup.PUT("/config/:key", UpdateEmbeddingConfig)
	}
}
