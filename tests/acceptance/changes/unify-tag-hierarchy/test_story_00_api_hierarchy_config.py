def test_config_read_write_cycle(api):
    resp = api.get("/api/hierarchy/config")
    assert resp["success"] is True
    original_templates = resp["data"]["templates"]

    modified = _modify_first_level_name(original_templates)
    resp = api.put("/api/hierarchy/config", json={"templates": modified})
    assert resp["success"] is True
    assert "impact" in resp["data"]

    resp = api.get("/api/hierarchy/config")
    assert resp["success"] is True
    _assert_level_name_changed(original_templates, resp["data"]["templates"])

    resp = api.put("/api/hierarchy/config", json={"templates": original_templates})
    assert resp["success"] is True

    resp = api.get("/api/hierarchy/config")
    assert resp["success"] is True
    assert resp["data"]["templates"] == original_templates


def _modify_first_level_name(templates):
    import copy
    templates = copy.deepcopy(templates)
    if templates and templates[0].get("levels"):
        templates[0]["levels"][0]["name"] += "_test"
    return templates


def _assert_level_name_changed(original, current):
    if original and original[0].get("levels"):
        assert current[0]["levels"][0]["name"] == original[0]["levels"][0]["name"] + "_test"
