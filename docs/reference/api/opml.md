# OPML 导入导出

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/import-opml` | 导入 OPML |
| GET | `/api/export-opml` | 导出 OPML |

---

### POST /api/import-opml

`multipart/form-data`，字段名 `file`，类型 `.opml` 或 `.xml`。

```json
{
  "success": true,
  "message": "Imported successfully",
  "data": {
    "feeds_added": 10,
    "categories_added": 3,
    "errors": [],
    "async_update": true
  }
}
```

### GET /api/export-opml

`Content-Type: text/xml`，`Content-Disposition: attachment; filename=feeds.opml`。
