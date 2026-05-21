from helpers.browser import navigate_to_tags
from helpers.selectors import TAGS_PAGE, TEMPLATE_DIALOG


def test_template_dialog_opens(page):
    navigate_to_tags(page)

    page.locator(TAGS_PAGE["settings_btn"]).click()
    page.wait_for_selector(TEMPLATE_DIALOG["overlay"])
    assert page.locator(TEMPLATE_DIALOG["overlay"]).is_visible()
    assert page.locator(TEMPLATE_DIALOG["level_card"]).first.is_visible()

    page.locator(TEMPLATE_DIALOG["cancel_btn"]).click()
    page.wait_for_selector(TEMPLATE_DIALOG["overlay"], state="hidden")
