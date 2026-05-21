from helpers.browser import navigate_to_tags
from helpers.selectors import TAGS_PAGE, TEXT


def test_tags_page_loads(page):
    navigate_to_tags(page)

    page.locator(TAGS_PAGE["title"])
    assert page.locator(TAGS_PAGE["title"]).is_visible()
    assert page.get_by_text(TEXT["page_title"], exact=True).is_visible()

    for label in [TEXT["category_event"], TEXT["category_person"], TEXT["category_keyword"]]:
        assert page.get_by_text(label, exact=True).is_visible()

    assert page.locator(TAGS_PAGE["sidebar"]).is_visible()
    assert page.locator(TAGS_PAGE["content"]).is_visible()
    assert page.locator(TAGS_PAGE["bottombar"]).is_visible()

    event_btn = page.get_by_text(TEXT["category_event"], exact=True)
    active_cls = TAGS_PAGE["category_btn_active"].lstrip(".")
    assert active_cls in event_btn.get_attribute("class")
