# 分类 Categories

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/categories` | 获取所有分类 |
| POST | `/api/categories` | 创建分类 |
| PUT | `/api/categories/:category_id` | 更新分类 |
| DELETE | `/api/categories/:category_id` | 删除分类 |

---

### GET /api/categories

获取所有分类，按名称升序，附带 `feed_count`。

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "技术",
      "slug": "a1b2c3d4",
      "icon": "folder",
      "color": "#6366f1",
      "description": "技术相关订阅",
      "created_at": "2025-01-15 10:30:00",
      "feed_count": 5
    }
  ]
}
```

### POST /api/categories

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 分类名称（唯一） |
| `slug` | string | 否 | URL slug，留空自动生成 |
| `icon` | string | 否 | 默认 `folder` |
| `color` | string | 否 | 默认 `#6366f1` |
| `description` | string | 否 | 描述 |

`201`：返回创建的分类。`409`：同名已存在。

### PUT /api/categories/:category_id

只更新请求体中提供的字段，同上但全部可选。

`200`：更新后的分类。`404`：不存在。

### DELETE /api/categories/:category_id

```json
{ "success": true, "message": "Category deleted successfully" }
```
