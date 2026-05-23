# Firecrawl集成测试设计文档

## 概述

设计并实现一个完整的Firecrawl集成测试脚本，验证Go后端的Firecrawl功能是否正常工作。

## 目标

- 验证Firecrawl服务连接
- 验证数据库配置正确性
- 验证后端API功能
- 验证完整的文章抓取流程
- 验证抓取结果质量

## 技术选型

- **语言**: Python 3
- **依赖**: requests, sqlite3
- **测试方式**: 线性集成测试（按实际使用流程顺序执行）

## 架构设计

### 项目结构

```
tests/
└── firecrawl/
    ├── test_firecrawl_integration.py    # 主测试脚本
    ├── config.py                        # 测试配置
    └── README.md                        # 测试说明文档
```

### 核心类设计

```python
class FirecrawlIntegrationTest:
    """Firecrawl完整集成测试"""
    
    def __init__(self):
        # 配置参数
        self.firecrawl_url = "http://192.168.5.27:3002"
        self.backend_url = "http://localhost:5000"
        self.db_path = "backend-go/rss_reader.db"
```

### 测试流程

```
配置验证 → Firecrawl连接测试 → 后端API测试 → 完整抓取流程 → 结果验证
```

## 详细设计

### 1. 配置设计（支持扩展）

```python
class TestConfig:
    # Firecrawl服务配置
    FIRECRAWL_BASE_URL = "http://192.168.5.27:3002"
    FIRECRAWL_TIMEOUT = 60
    
    # 后端服务配置
    BACKEND_BASE_URL = "http://localhost:5000"
    BACKEND_TIMEOUT = 60
    
    # 数据库配置
    DATABASE_PATH = "backend-go/rss_reader.db"
    
    # 测试文章配置
    TEST_ARTICLE_ID = 4
    TEST_ARTICLE_URL = "https://sspai.com/post/105308"
    
    # 扩展配置区域
    DENOISE_ENABLED = False
    PROCESSING_PIPELINE = ["crawl"]  # 可扩展
```

### 2. 工具方法设计

- `_http_request()`: 统一的HTTP请求封装
- `_db_query()`: 数据库查询封装
- `_verify_response()`: 响应验证工具
- `_cleanup_test_data()`: 清理测试数据
- `_execute_test_step()`: 测试步骤执行器

### 3. 测试步骤实现

#### 步骤1：配置验证
- 检查ai_settings表中的firecrawl配置
- 验证api_url、enabled等关键字段

#### 步骤2：Firecrawl连接测试
- 直接测试Firecrawl API
- 使用测试文章URL进行抓取
- 验证返回结果

#### 步骤3：后端API测试
- GET /api/firecrawl/status
- POST /api/firecrawl/feed/{id}/enable

#### 步骤4：完整抓取流程
- POST /api/firecrawl/article/{id}
- 轮询数据库状态等待完成
- 验证firecrawl_status、firecrawl_content、firecrawl_crawled_at

#### 步骤5：结果验证
- 验证内容长度和质量
- 验证内容格式
- 预留去噪等扩展功能接口

### 4. 测试报告生成

```python
class TestReport:
    def add_step(step_name, success, duration, details)
    def generate_report()  # 生成详细测试报告
```

## 扩展性设计

### 预留扩展点

1. **去噪功能**: test_5_result_verification中预留去噪测试接口
2. **处理管道**: PROCESSING_PIPELINE支持添加新功能
3. **配置扩展**: config.py中预留扩展配置区域
4. **测试步骤**: _execute_test_step提供统一的测试框架

## 测试数据

- **文章ID**: 4
- **文章URL**: https://sspai.com/post/105308
- **文章标题**: 丢掉遥控器，寻找生命感：这是我的 Vbot「大头」机器狗使用体验

## 成功标准

- [ ] Firecrawl服务连接成功
- [ ] 数据库配置正确
- [ ] 后端API响应正常
- [ ] 文章抓取成功（status=completed）
- [ ] 抓取内容长度 > 1000字符
- [ ] 内容包含预期关键词

## 后续扩展

- 去噪功能测试
- 内容摘要功能测试
- 批量抓取测试
- 错误处理测试
- 性能压测