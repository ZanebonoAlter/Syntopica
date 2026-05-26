package ws

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"syntopica-backend/internal/platform/logging"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（开发环境）
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Hub WebSocket连接管理中心
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Client WebSocket客户端
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// JobUpdate 单个任务更新
type JobUpdate struct {
	ID           string `json:"id"`
	FeedID       *uint  `json:"feed_id"`
	FeedName     string `json:"feed_name"`
	FeedIcon     string `json:"feed_icon"`
	FeedColor    string `json:"feed_color"`
	CategoryID   *uint  `json:"category_id"`
	CategoryName string `json:"category_name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ResultID     *uint  `json:"result_id,omitempty"`
}

// FirecrawlProgressMessage Firecrawl进度消息
type FirecrawlProgressMessage struct {
	Type      string                    `json:"type"`
	BatchID   string                    `json:"batch_id"`
	Status    string                    `json:"status"` // processing/completed
	Total     int                       `json:"total"`
	Completed int                       `json:"completed"`
	Failed    int                       `json:"failed"`
	Current   *FirecrawlArticleProgress `json:"current,omitempty"`
}

// FirecrawlArticleProgress 单篇文章抓取进度
type FirecrawlArticleProgress struct {
	ID     uint   `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // processing/completed/failed
	Error  string `json:"error,omitempty"`
}

// TagCompletedMessage 标签完成通知消息
type TagCompletedMessage struct {
	Type      string             `json:"type"` // "tag_completed"
	ArticleID uint               `json:"article_id"`
	JobID     uint               `json:"job_id"`
	Tags      []TagCompletedItem `json:"tags"`
}

// TagCompletedItem 单个标签信息
type TagCompletedItem struct {
	Slug     string  `json:"slug"`
	Label    string  `json:"label"`
	Category string  `json:"category"`
	Score    float64 `json:"score"`
	Icon     string  `json:"icon"`
}

// TagFailedMessage 标签任务失败通知消息
type TagFailedMessage struct {
	Type      string `json:"type"` // "tag_failed"
	ArticleID uint   `json:"article_id"`
	JobID     uint   `json:"job_id"`
	Error     string `json:"error"`
}

// AutoRefreshCompleteMessage Auto-refresh 完成通知消息
type AutoRefreshCompleteMessage struct {
	Type            string  `json:"type"`
	TriggeredFeeds  int     `json:"triggered_feeds"`
	StaleResetFeeds int     `json:"stale_reset_feeds"`
	DurationSeconds float64 `json:"duration_seconds"`
	Timestamp       string  `json:"timestamp"`
}

// OrganizeProgressMessage 标签整理进度消息
type OrganizeProgressMessage struct {
	Type              string              `json:"type"`
	Status            string              `json:"status"`
	TotalUnclassified int                 `json:"total_unclassified"`
	Processed         int                 `json:"processed"`
	CurrentGroup      *OrganizeGroupInfo  `json:"current_group,omitempty"`
	Groups            []OrganizeGroupInfo `json:"groups,omitempty"`
	Category          string              `json:"category,omitempty"`
}

// OrganizeGroupInfo 单个整理分组信息
type OrganizeGroupInfo struct {
	NewLabel       string  `json:"new_label"`
	CandidateCount int     `json:"candidate_count"`
	Action         string  `json:"action"`
	Similarity     float64 `json:"similarity,omitempty"`
}

var hubInstance *Hub
var hubOnce sync.Once

// GetHub 获取Hub单例
func GetHub() *Hub {
	hubOnce.Do(func() {
		hubInstance = &Hub{
			clients:    make(map[*Client]bool),
			broadcast:  make(chan []byte, 256),
			register:   make(chan *Client),
			unregister: make(chan *Client),
		}
		go hubInstance.run()
	})
	return hubInstance
}

// run 启动Hub主循环
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logging.Infof("WebSocket客户端已连接，当前连接数: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			logging.Infof("WebSocket客户端已断开，当前连接数: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := make(map[*Client]bool, len(h.clients))
			for client := range h.clients {
				clients[client] = true
			}
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.send <- message:
				default:
					// 客户端发送缓冲区满，关闭连接
					h.mu.Lock()
					delete(h.clients, client)
					close(client.send)
					h.mu.Unlock()
					_ = client.conn.Close()
				}
			}
		}
	}
}

// BroadcastRaw 广播原始JSON数据
func (h *Hub) BroadcastRaw(data []byte) {
	select {
	case h.broadcast <- data:
	default:
		logging.Warnf("广播通道已满，丢弃消息")
	}
}

// HandleWebSocket WebSocket连接处理器
func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logging.Warnf("WebSocket升级失败: %v", err)
		return
	}

	hub := GetHub()
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	hub.register <- client

	// 启动读写goroutine
	go client.writePump()
	go client.readPump()
}

// readPump 读取客户端消息（保持连接活跃）
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	// 设置 pong 处理器保持连接
	c.conn.SetPongHandler(func(string) error {
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logging.Warnf("WebSocket读取错误: %v", err)
			}
			break
		}
		// 客户端发送的消息可以在这里处理（如ping/pong）
	}
}

// writePump 向客户端写入消息
func (c *Client) writePump() {
	defer func() {
		_ = c.conn.Close()
	}()

	for message := range c.send {
		_ = c.conn.SetWriteDeadline(getDeadline())
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			logging.Warnf("WebSocket写入失败: %v", err)
			return
		}
	}
}

// getDeadline 获取写入超时时间
func getDeadline() time.Time {
	return time.Now().Add(60 * time.Second)
}
