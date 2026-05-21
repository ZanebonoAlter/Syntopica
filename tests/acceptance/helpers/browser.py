import os

import pytest
from playwright.sync_api import sync_playwright

FRONTEND_URL = "http://localhost:3000"

HEADLESS = os.environ.get("HEADLESS", "true").lower() == "true"


@pytest.fixture(scope="session")
def playwright_instance():
    with sync_playwright() as p:
        yield p


@pytest.fixture(scope="session")
def browser(playwright_instance):
    b = playwright_instance.chromium.launch(headless=HEADLESS)
    yield b
    b.close()


@pytest.fixture
def context(browser):
    ctx = browser.new_context(viewport={"width": 1280, "height": 720})
    yield ctx
    ctx.close()


@pytest.fixture
def page(context):
    p = context.new_page()
    yield p
    p.close()


def navigate_to_tags(page):
    page.goto(f"{FRONTEND_URL}/tags", wait_until="networkidle")
    return page
