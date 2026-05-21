from helpers.browser import navigate_to_tags
from helpers.selectors import SECTOR_LIST, TAGS_PAGE, TEXT


def test_sector_list_visible(page):
    navigate_to_tags(page)

    assert page.locator(SECTOR_LIST["container"]).is_visible()
    assert page.get_by_text(TEXT["all"], exact=True).is_visible()
    assert page.locator(SECTOR_LIST["add_btn"]).is_visible()
    assert page.locator(SECTOR_LIST["regenerate_btn"]).is_visible()


def test_category_switch(page):
    navigate_to_tags(page)

    person_btn = page.get_by_text(TEXT["category_person"], exact=True)
    person_btn.click()
    active_cls = TAGS_PAGE["category_btn_active"].lstrip(".")
    assert active_cls in person_btn.get_attribute("class")

    event_btn = page.get_by_text(TEXT["category_event"], exact=True)
    event_btn.click()
    assert active_cls in event_btn.get_attribute("class")
