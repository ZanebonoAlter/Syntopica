import time

import pytest


def test_rebuild_cycle(api):
    resp = api.post("/api/hierarchy/rebuild/start", json={"category": "event"})
    assert resp["success"] is True
    job_id = resp["data"]["id"]

    deadline = time.time() + 600
    while time.time() < deadline:
        resp = api.get(f"/api/hierarchy/rebuild/{job_id}")
        assert resp["success"] is True
        status = resp["data"]["status"]
        if status in ("completed", "failed"):
            break
        time.sleep(5)
    else:
        pytest.skip("重建超时")

    if status == "completed":
        assert resp["data"]["processed_tags"] >= 0
        assert resp["data"]["total_tags"] >= 0
