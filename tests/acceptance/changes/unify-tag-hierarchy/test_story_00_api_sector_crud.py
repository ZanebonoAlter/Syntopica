def test_sector_crud_cycle(api):
    resp = api.post("/api/narratives/board-concepts", json={
        "name": "验收测试板块",
        "category": "event",
        "source": "manual",
    })
    assert resp["success"] is True
    assert resp["data"]["source"] == "manual"
    assert resp["data"]["protected"] is True
    sector_id = resp["data"]["id"]

    resp = api.get("/api/narratives/board-concepts", params={"category": "event"})
    assert resp["success"] is True
    ids = [s["id"] for s in resp["data"]]
    assert sector_id in ids

    resp = api.delete(f"/api/narratives/board-concepts/{sector_id}", params={"confirm": "true"})
    assert resp["success"] is True

    resp = api.get("/api/narratives/board-concepts", params={"category": "event"})
    assert resp["success"] is True
    ids = [s["id"] for s in resp["data"]]
    assert sector_id not in ids
