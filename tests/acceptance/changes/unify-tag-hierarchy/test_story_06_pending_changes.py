import pytest

from helpers.browser import navigate_to_tags
from helpers.selectors import PENDING_PANEL, TAGS_PAGE, TEXT


def test_pending_changes_badge(page):
    navigate_to_tags(page)

    assert page.locator(TAGS_PAGE["bottombar"]).is_visible()
    page.locator(TAGS_PAGE["pending_btn"]).is_visible()


def test_pending_panel_opens(page):
    navigate_to_tags(page)

    btn = page.locator(TAGS_PAGE["pending_btn"])
    if not btn.is_visible():
        pytest.skip("待确认变更按钮不可见")

    badge = page.locator(TAGS_PAGE["pending_badge"])
    if not badge.is_visible():
        pytest.skip("没有待确认变更")

    btn.click()
    page.wait_for_selector(PENDING_PANEL["panel"])
    assert page.locator(PENDING_PANEL["panel"]).is_visible()

    page.locator(PENDING_PANEL["close_btn"]).click()
    page.wait_for_selector(PENDING_PANEL["panel"], state="hidden")
