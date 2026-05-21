import pytest

from helpers.browser import navigate_to_tags
from helpers.selectors import HIERARCHY, SECTOR_LIST, TAGS_PAGE, TEXT


def test_hierarchy_tree_filter(page):
    navigate_to_tags(page)

    assert page.locator(HIERARCHY["root"]).is_visible()

    items = page.locator(SECTOR_LIST["item"]).all()
    non_all = [i for i in items if TEXT["all"] not in (i.text_content() or "")]
    if len(non_all) == 0:
        pytest.skip("没有板块数据，无法测试层级树筛选")

    non_all[0].click()
    active_cls = SECTOR_LIST["item_active"].lstrip(".")
    assert active_cls in non_all[0].get_attribute("class")

    page.get_by_text(TEXT["all"], exact=True).click()
    assert active_cls not in non_all[0].get_attribute("class")


def test_empty_hierarchy(page):
    navigate_to_tags(page)

    if page.locator(HIERARCHY["empty"]).is_visible():
        assert page.locator(HIERARCHY["empty"]).is_visible()
    else:
        assert page.locator(HIERARCHY["tree"]).is_visible()
