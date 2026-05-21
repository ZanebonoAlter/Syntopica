import time

from helpers.api import APIClient
from helpers.browser import navigate_to_tags
from helpers.selectors import ADD_SECTOR_DIALOG, SECTOR_LIST, TEXT


def test_create_sector(page):
    navigate_to_tags(page)

    page.locator(SECTOR_LIST["add_btn"]).click()
    page.wait_for_selector(ADD_SECTOR_DIALOG["overlay"])
    assert page.locator(ADD_SECTOR_DIALOG["overlay"]).is_visible()

    label = f"验收测试板块{int(time.time())}"
    page.locator(ADD_SECTOR_DIALOG["input"]).fill(label)
    page.get_by_text(TEXT["confirm_add"], exact=True).click()

    page.wait_for_selector(ADD_SECTOR_DIALOG["overlay"], state="hidden")
    assert not page.locator(ADD_SECTOR_DIALOG["overlay"]).is_visible()

    page.wait_for_selector(SECTOR_LIST["item_label"])
    labels = page.locator(SECTOR_LIST["item_label"]).all_text_contents()
    assert label in labels

    api = APIClient()
    resp = api.get("/api/narratives/board-concepts", params={"category": "event"})
    for s in resp.get("data", []):
        if s["name"] == label:
            api.delete(f"/api/narratives/board-concepts/{s['id']}", params={"confirm": "true"})
            break
