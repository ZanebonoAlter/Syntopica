import pytest


def test_pending_changes_cycle(api):
    resp = api.get("/api/hierarchy/pending")
    assert resp["success"] is True
    before = resp["data"]

    if not before:
        pytest.skip("无待处理变更")

    resp = api.post("/api/hierarchy/pending/approve", json={"approve_all": True})
    assert resp["success"] is True

    resp = api.get("/api/hierarchy/pending")
    assert resp["success"] is True
    assert len(resp["data"]) <= len(before)
