# Firecrawl集成测试实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 创建完整的Python测试脚本，验证Go后端的Firecrawl功能是否正常工作

**Architecture:** 线性集成测试脚本，按实际使用流程顺序执行5个测试步骤：配置验证 → Firecrawl连接 → 后端API测试 → 完整抓取流程 → 结果验证

**Tech Stack:** Python 3, requests库, sqlite3, unittest框架

---

## Task 1: 创建测试目录和配置文件

**Files:**
- Create: `tests/firecrawl/__init__.py`
- Create: `tests/firecrawl/config.py`
- Create: `tests/firecrawl/README.md`

**Step 1: 创建测试目录结构**

```bash
mkdir -p tests/firecrawl
touch tests/firecrawl/__init__.py
```

**Step 2: 编写配置文件**

创建 `tests/firecrawl/config.py`:

```python
"""Firecrawl集成测试配置"""


class TestConfig:
    """测试配置 - 支持多功能扩展"""
    
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
    
    # ============ 扩展配置区域 ============
    # 未来的去噪功能
    DENOISE_ENABLED = False
    DENOISE_CONFIG = {
        "remove_ads": True,
        "remove_navigation": True,
        "min_content_length": 100
    }
    
    # 未来的其他处理功能
    PROCESSING_PIPELINE = ["crawl"]  # 可扩展为 ["crawl", "denoise", "summarize"]
```

**Step 3: 编写README文档**

创建 `tests/firecrawl/README.md`:

```markdown
# Firecrawl集成测试

## 测试目标

验证Go后端的Firecrawl功能是否正常工作，包括：
- Firecrawl服务连接
- 数据库配置正确性
- 后端API功能
- 完整的文章抓取流程
- 抓取结果质量

## 运行测试

```bash
# 确保后端服务运行
cd backend-go
go run cmd/server/main.go

# 在另一个终端运行测试
cd tests/firecrawl
python test_firecrawl_integration.py
```

## 测试步骤

1. 配置验证 - 检查数据库中的Firecrawl配置
2. Firecrawl连接测试 - 直接测试Firecrawl API
3. 后端API测试 - 测试Go后端的Firecrawl相关API
4. 完整抓取流程 - 通过后端API触发完整抓取
5. 结果验证 - 验证抓取内容质量

## 扩展功能

测试脚本预留了扩展接口，支持未来添加：
- 去噪功能测试
- 内容摘要功能测试
- 批量抓取测试
```

**Step 4: 提交配置文件**

```bash
git add tests/firecrawl/
git commit -m "test: add firecrawl integration test configuration"
```

---

## Task 2: 创建测试基类和工具方法

**Files:**
- Create: `tests/firecrawl/test_firecrawl_integration.py`

**Step 1: 编写测试基类框架**

创建 `tests/firecrawl/test_firecrawl_integration.py`:

```python
#!/usr/bin/env python3
"""Firecrawl完整集成测试脚本"""

import json
import sqlite3
import time
from pathlib import Path
from typing import Dict, Any, Optional, List

import requests

from config import TestConfig


class TestReport:
    """测试报告生成器"""
    
    def __init__(self):
        self.steps: List[Dict[str, Any]] = []
        self.start_time: Optional[float] = None
        self.end_time: Optional[float] = None
        
    def start(self):
        """开始测试"""
        self.start_time = time.time()
        
    def add_step(self, step_name: str, success: bool, duration: float, details: str):
        """记录测试步骤结果"""
        self.steps.append({
            "step": step_name,
            "success": success,
            "duration": duration,
            "details": details
        })
        
    def finish(self):
        """结束测试"""
        self.end_time = time.time()
        
    def generate_report(self) -> str:
        """生成测试报告"""
        report = []
        report.append("=" * 70)
        report.append("📊 Firecrawl集成测试报告")
        report.append("=" * 70)
        
        if self.start_time and self.end_time:
            total_duration = self.end_time - self.start_time
            report.append(f"\n总耗时: {total_duration:.2f}秒\n")
        
        for idx, step in enumerate(self.steps, 1):
            status = "✅" if step["success"] else "❌"
            report.append(f"{idx}. {step['step']}: {status}")
            report.append(f"   耗时: {step['duration']:.2f}秒")
            report.append(f"   详情: {step['details']}")
            report.append("")
        
        passed = sum(1 for s in self.steps if s["success"])
        total = len(self.steps)
        report.append("=" * 70)
        report.append(f"测试结果: {passed}/{total} 通过")
        report.append("=" * 70)
        
        return "\n".join(report)


class FirecrawlIntegrationTest:
    """Firecrawl完整集成测试"""
    
    def __init__(self):
        self.config = TestConfig()
        self.report = TestReport()
        
    # ============ 工具方法 ============
    def _http_request(
        self, 
        method: str, 
        url: str, 
        timeout: int = 60,
        **kwargs
    ) -> requests.Response:
        """统一的HTTP请求封装"""
        kwargs.setdefault('timeout', timeout)
        response = requests.request(method, url, **kwargs)
        return response
        
    def _db_query(self, sql: str, params: Optional[tuple] = None) -> List[tuple]:
        """数据库查询封装"""
        db_path = Path(self.config.DATABASE_PATH)
        if not db_path.exists():
            raise FileNotFoundError(f"数据库文件不存在: {db_path}")
            
        conn = sqlite3.connect(str(db_path))
        cursor = conn.cursor()
        
        try:
            if params:
                cursor.execute(sql, params)
            else:
                cursor.execute(sql)
            results = cursor.fetchall()
            return results
        finally:
            conn.close()
            
    def _db_execute(self, sql: str, params: Optional[tuple] = None):
        """数据库执行（更新/插入）封装"""
        db_path = Path(self.config.DATABASE_PATH)
        conn = sqlite3.connect(str(db_path))
        cursor = conn.cursor()
        
        try:
            if params:
                cursor.execute(sql, params)
            else:
                cursor.execute(sql)
            conn.commit()
        finally:
            conn.close()
            
    def _verify_response(
        self, 
        response: requests.Response, 
        expected_status: int = 200
    ) -> Dict[str, Any]:
        """响应验证工具"""
        if response.status_code != expected_status:
            raise AssertionError(
                f"HTTP状态码错误: 期望{expected_status}, 实际{response.status_code}"
            )
            
        data = response.json()
        if not data.get("success"):
            error = data.get("error", "未知错误")
            raise AssertionError(f"API返回失败: {error}")
            
        return data
        
    def _execute_test_step(self, step_name: str, test_func) -> bool:
        """测试步骤执行器"""
        start = time.time()
        success = False
        details = ""
        
        try:
            test_func()
            success = True
            details = "测试通过"
        except Exception as e:
            details = f"测试失败: {str(e)}"
            
        duration = time.time() - start
        self.report.add_step(step_name, success, duration, details)
        return success
```

**Step 2: 提交测试基类**

```bash
git add tests/firecrawl/test_firecrawl_integration.py
git commit -m "test: add firecrawl integration test base class"
```

---

## Task 3: 实现测试步骤1和2

**Files:**
- Modify: `tests/firecrawl/test_firecrawl_integration.py`

**Step 1: 添加测试步骤1 - 配置验证**

在 `FirecrawlIntegrationTest` 类中添加方法：

```python
    def test_1_config_validation(self):
        """
        步骤1：配置验证
        验证数据库中的Firecrawl配置是否正确
        """
        # 1. 检查ai_settings表是否有firecrawl配置
        sql = "SELECT value FROM ai_settings WHERE key='summary_config'"
        results = self._db_query(sql)
        
        if not results:
            raise AssertionError("未找到summary_config配置")
            
        # 2. 解析JSON配置
        config_json = results[0][0]
        config = json.loads(config_json)
        
        # 3. 检查firecrawl配置是否存在
        if 'firecrawl' not in config:
            raise AssertionError("firecrawl配置缺失，请先在前端配置Firecrawl")
            
        firecrawl_config = config['firecrawl']
        
        # 4. 验证关键字段
        if not firecrawl_config.get('enabled'):
            raise AssertionError("Firecrawl未启用")
            
        api_url = firecrawl_config.get('api_url', '')
        if not api_url:
            raise AssertionError("api_url未配置")
            
        # 5. 记录配置信息
        print(f"✓ Firecrawl配置验证通过")
        print(f"  API地址: {api_url}")
        print(f"  模式: {firecrawl_config.get('mode', 'scrape')}")
```

**Step 2: 添加测试步骤2 - Firecrawl连接测试**

在 `FirecrawlIntegrationTest` 类中添加方法：

```python
    def test_2_firecrawl_connection(self):
        """
        步骤2：Firecrawl服务连接测试
        直接测试Firecrawl API是否可用
        """
        # 1. 构造请求
        url = f"{self.config.FIRECRAWL_BASE_URL}/v1/scrape"
        payload = {
            "url": self.config.TEST_ARTICLE_URL,
            "formats": ["markdown"]
        }
        
        print(f"✓ 测试Firecrawl连接: {url}")
        print(f"  测试文章: {self.config.TEST_ARTICLE_URL}")
        
        # 2. 发送请求
        response = self._http_request(
            "POST",
            url,
            json=payload,
            timeout=self.config.FIRECRAWL_TIMEOUT
        )
        
        # 3. 验证响应
        if response.status_code != 200:
            raise AssertionError(
                f"Firecrawl API错误: HTTP {response.status_code}"
            )
            
        data = response.json()
        
        if not data.get("success"):
            error = data.get("error", "未知错误")
            raise AssertionError(f"Firecrawl抓取失败: {error}")
            
        # 4. 验证返回内容
        markdown_content = data.get("data", {}).get("markdown", "")
        if not markdown_content:
            raise AssertionError("Firecrawl返回内容为空")
            
        content_length = len(markdown_content)
        print(f"✓ Firecrawl连接测试通过")
        print(f"  抓取内容长度: {content_length} 字符")
```

**Step 3: 提交测试步骤**

```bash
git add tests/firecrawl/test_firecrawl_integration.py
git commit -m "test: add config validation and firecrawl connection tests"
```

---

## Task 4: 实现测试步骤3 - 后端API测试

**Files:**
- Modify: `tests/firecrawl/test_firecrawl_integration.py`

**Step 1: 添加测试步骤3 - 后端API测试**

在 `FirecrawlIntegrationTest` 类中添加方法：

```python
    def test_3_backend_api(self):
        """
        步骤3：后端API测试
        测试Go后端的Firecrawl相关API是否正常
        """
        base_url = self.config.BACKEND_BASE_URL
        
        # 子测试1：GET /api/firecrawl/status
        print("✓ 测试 GET /api/firecrawl/status")
        status_url = f"{base_url}/api/firecrawl/status"
        response = self._http_request("GET", status_url)
        
        data = self._verify_response(response)
        status_data = data.get("data", {})
        
        if not status_data.get("enabled"):
            raise AssertionError("后端Firecrawl未启用")
            
        print(f"  API地址: {status_data.get('api_url')}")
        
        # 子测试2：POST /api/firecrawl/feed/{id}/enable
        # 先获取测试文章所属的feed_id
        sql = f"SELECT feed_id FROM articles WHERE id = {self.config.TEST_ARTICLE_ID}"
        results = self._db_query(sql)
        
        if not results:
            raise AssertionError(f"未找到文章ID: {self.config.TEST_ARTICLE_ID}")
            
        feed_id = results[0][0]
        print(f"✓ 测试 POST /api/firecrawl/feed/{feed_id}/enable")
        
        enable_url = f"{base_url}/api/firecrawl/feed/{feed_id}/enable"
        response = self._http_request(
            "POST",
            enable_url,
            json={"enabled": True}
        )
        
        data = self._verify_response(response)
        print(f"  Feed Firecrawl已启用")
        
        # 验证数据库中feed的firecrawl_enabled字段已更新
        sql = f"SELECT firecrawl_enabled FROM feeds WHERE id = {feed_id}"
        results = self._db_query(sql)
        
        if not results or not results[0][0]:
            raise AssertionError("Feed的firecrawl_enabled未正确更新")
            
        print("✓ 后端API测试通过")
```

**Step 2: 提交测试步骤**

```bash
git add tests/firecrawl/test_firecrawl_integration.py
git commit -m "test: add backend API test"
```

---

## Task 5: 实现测试步骤4 - 完整抓取流程

**Files:**
- Modify: `tests/firecrawl/test_firecrawl_integration.py`

**Step 1: 添加测试步骤4 - 完整抓取流程**

在 `FirecrawlIntegrationTest` 类中添加方法：

```python
    def test_4_full_crawl_flow(self):
        """
        步骤4：完整抓取流程测试
        通过后端API触发完整的文章抓取流程
        """
        article_id = self.config.TEST_ARTICLE_ID
        
        # 1. 确保文章的firecrawl_enabled为True
        sql = f"UPDATE articles SET firecrawl_enabled = 1, firecrawl_status = 'pending' WHERE id = {article_id}"
        self._db_execute(sql)
        
        print(f"✓ 触发文章抓取: ID={article_id}")
        
        # 2. 调用后端API触发抓取
        crawl_url = f"{self.config.BACKEND_BASE_URL}/api/firecrawl/article/{article_id}"
        response = self._http_request("POST", crawl_url, timeout=120)
        
        data = self._verify_response(response)
        
        # 3. 等待抓取完成（轮询数据库状态）
        print("  等待抓取完成...")
        max_wait = 60  # 最多等待60秒
        check_interval = 2
        elapsed = 0
        
        while elapsed < max_wait:
            sql = f"SELECT firecrawl_status, firecrawl_error FROM articles WHERE id = {article_id}"
            results = self._db_query(sql)
            
            if not results:
                raise AssertionError(f"文章ID {article_id} 不存在")
                
            status = results[0][0]
            error = results[0][1]
            
            if status == "completed":
                print(f"  抓取完成 (耗时: {elapsed}秒)")
                break
            elif status == "failed":
                raise AssertionError(f"抓取失败: {error}")
                
            time.sleep(check_interval)
            elapsed += check_interval
        else:
            raise AssertionError("抓取超时")
            
        # 4. 验证数据库字段更新
        sql = f"""
            SELECT firecrawl_status, firecrawl_content, firecrawl_crawled_at
            FROM articles 
            WHERE id = {article_id}
        """
        results = self._db_query(sql)
        
        if not results:
            raise AssertionError("无法读取文章数据")
            
        status, content, crawled_at = results[0]
        
        if status != "completed":
            raise AssertionError(f"状态错误: {status}")
            
        if not content:
            raise AssertionError("抓取内容为空")
            
        if not crawled_at:
            raise AssertionError("firecrawl_crawled_at未更新")
            
        print(f"✓ 完整抓取流程测试通过")
        print(f"  内容长度: {len(content)} 字符")
        print(f"  抓取时间: {crawled_at}")
```

**Step 2: 提交测试步骤**

```bash
git add tests/firecrawl/test_firecrawl_integration.py
git commit -m "test: add full crawl flow test"
```

---

## Task 6: 实现测试步骤5和主函数

**Files:**
- Modify: `tests/firecrawl/test_firecrawl_integration.py`

**Step 1: 添加测试步骤5 - 结果验证**

在 `FirecrawlIntegrationTest` 类中添加方法：

```python
    def test_5_result_verification(self):
        """
        步骤5：结果验证
        详细验证抓取到的内容质量
        """
        article_id = self.config.TEST_ARTICLE_ID
        
        # 1. 从数据库读取firecrawl_content
        sql = f"SELECT firecrawl_content FROM articles WHERE id = {article_id}"
        results = self._db_query(sql)
        
        if not results or not results[0][0]:
            raise AssertionError("无法读取抓取内容")
            
        content = results[0][0]
        
        # 2. 验证内容长度
        if len(content) < 1000:
            raise AssertionError(
                f"内容长度不足: {len(content)} 字符 (期望 > 1000)"
            )
            
        print(f"✓ 内容长度验证通过: {len(content)} 字符")
        
        # 3. 验证内容包含预期关键词
        # 从测试文章URL中提取标题关键词
        expected_keywords = ["Vbot", "大头", "机器人"]
        
        missing_keywords = []
        for keyword in expected_keywords:
            if keyword not in content:
                missing_keywords.append(keyword)
                
        if missing_keywords:
            raise AssertionError(
                f"内容缺少关键词: {', '.join(missing_keywords)}"
            )
            
        print(f"✓ 关键词验证通过")
        
        # 4. 验证内容格式为Markdown
        markdown_indicators = ["#", "**", "-", "["]
        has_markdown = any(indicator in content for indicator in markdown_indicators)
        
        if not has_markdown:
            raise AssertionError("内容格式不符合Markdown")
            
        print(f"✓ Markdown格式验证通过")
        
        # 5. [扩展] 调用去噪功能（如果启用）
        if self.config.DENOISE_ENABLED:
            print("  [扩展] 执行去噪处理...")
            # 预留去噪功能接口
            # denoised_content = self._denoise_content(content)
            # 验证去噪后的内容质量
            
        print("✓ 结果验证测试通过")
```

**Step 2: 添加主函数**

在文件末尾添加：

```python
    def run_all_tests(self):
        """运行所有测试"""
        print("=" * 70)
        print("🚀 开始Firecrawl集成测试")
        print("=" * 70)
        print()
        
        self.report.start()
        
        # 按顺序执行测试
        tests = [
            ("配置验证", self.test_1_config_validation),
            ("Firecrawl连接测试", self.test_2_firecrawl_connection),
            ("后端API测试", self.test_3_backend_api),
            ("完整抓取流程", self.test_4_full_crawl_flow),
            ("结果验证", self.test_5_result_verification),
        ]
        
        for test_name, test_func in tests:
            print(f"\n▶ 执行测试: {test_name}")
            print("-" * 70)
            success = self._execute_test_step(test_name, test_func)
            
            if not success:
                print(f"\n⚠️  测试失败，停止后续测试")
                break
            print()
        
        self.report.finish()
        
        # 生成并打印报告
        report_text = self.report.generate_report()
        print("\n" + report_text)
        
        # 返回是否所有测试通过
        return all(step["success"] for step in self.report.steps)


def main():
    """主函数"""
    try:
        # 检查依赖
        import requests
    except ImportError:
        print("❌ 缺少依赖: requests")
        print("请运行: pip install requests")
        return 1
        
    # 运行测试
    tester = FirecrawlIntegrationTest()
    success = tester.run_all_tests()
    
    return 0 if success else 1


if __name__ == "__main__":
    exit(main())
```

**Step 3: 提交测试步骤**

```bash
git add tests/firecrawl/test_firecrawl_integration.py
git commit -m "test: add result verification and main function"
```

---

## Task 7: 配置数据库并运行测试

**Files:**
- None (数据库操作)

**Step 1: 更新数据库配置**

```bash
# 添加firecrawl配置到ai_settings表
sqlite3 backend-go/rss_reader.db "UPDATE ai_settings SET value = json_set(value, '$.firecrawl', json_object('enabled', true, 'api_url', 'http://192.168.5.27:3002', 'api_key', '', 'mode', 'scrape', 'timeout', 60, 'max_content_length', 50000)) WHERE key = 'summary_config';"
```

**Step 2: 验证配置**

```bash
sqlite3 backend-go/rss_reader.db "SELECT value FROM ai_settings WHERE key='summary_config';"
```

Expected output: JSON with firecrawl configuration

**Step 3: 安装Python依赖**

```bash
pip install requests
```

**Step 4: 启动Go后端服务**

在第一个终端：

```bash
cd backend-go
go run cmd/server/main.go
```

Expected: Server running on :5000

**Step 5: 运行测试脚本**

在第二个终端：

```bash
cd tests/firecrawl
python test_firecrawl_integration.py
```

Expected: All 5 tests pass with ✅

**Step 6: 提交最终版本**

```bash
git add tests/firecrawl/
git commit -m "test: complete firecrawl integration test implementation"
```

---

## 测试成功标准

- [x] 所有5个测试步骤通过
- [x] Firecrawl服务连接成功
- [x] 数据库配置正确
- [x] 后端API响应正常
- [x] 文章抓取成功（status=completed）
- [x] 抓取内容长度 > 1000字符
- [x] 内容包含预期关键词

## 后续扩展

- 添加去噪功能测试
- 添加内容摘要功能测试
- 添加批量抓取测试
- 添加错误处理测试
- 添加性能压测