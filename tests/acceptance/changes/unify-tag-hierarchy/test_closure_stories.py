import pytest

from helpers.api import APIClient
from helpers.browser import navigate_to_tags


@pytest.fixture
def api_client():
    return APIClient()


def _ok(response):
    assert response.get("success") is True, response
    return response.get("data")


def test_story_01_tags_closure_status_sector_bootstrap_and_tree_refresh(api_client, page):
    status = _ok(api_client.get("/api/hierarchy/closure-status?category=event"))
    assert "unplaced_tag_count" in status
    assert "blocker_counts" in status

    sector = _ok(api_client.post(
        "/api/narratives/board-concepts",
        json={"name": "验收测试板块", "description": "用于验证标签闭环刷新", "category": "event", "source": "manual"},
    ))
    try:
        navigate_to_tags(page)
        page.get_by_text("层级闭环").wait_for(timeout=10_000)
        page.get_by_text("验收测试板块").wait_for(timeout=10_000)
    finally:
        api_client.delete(f"/api/narratives/board-concepts/{sector['id']}?confirm=true")


def test_story_02_template_preview_cancel_then_apply_starts_rebuild(api_client):
    config = _ok(api_client.get("/api/hierarchy/config"))
    templates = config["templates"]

    preview = _ok(api_client.post(
        "/api/hierarchy/config/preview",
        json={"templates": templates, "change_log": "acceptance preview"},
    ))
    assert preview["preview_only"] is True

    after_preview = _ok(api_client.get("/api/hierarchy/config"))
    assert after_preview["version"] == config["version"]

    applied = _ok(api_client.put(
        "/api/hierarchy/config",
        json={"templates": templates, "change_log": "acceptance apply", "mode": "apply", "apply": True},
    ))
    assert applied["preview_only"] is False
    assert "rebuild_jobs" in applied


def test_story_03_llm_sector_partial_failure_returns_backend_result(api_client):
    result = _ok(api_client.post(
        "/api/narratives/board-concepts/regenerate/confirm",
        json={
            "category": "event",
            "diff": {
                "keep": [],
                "add": [],
                "merge": [{"source_ids": [999998], "target_id": 999999, "name": "缺失目标"}],
                "split": [],
                "affected_tag_count": 0,
            },
        },
    ))
    assert result["failed_count"] == 1
    assert result["results"][0]["operation"] == "merge"
    assert result["results"][0]["status"] == "failed"
    assert result["results"][0]["error"]
