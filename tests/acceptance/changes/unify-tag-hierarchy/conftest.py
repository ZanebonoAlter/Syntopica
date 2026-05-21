import pytest
from helpers.api import APIClient


@pytest.fixture
def api():
    return APIClient()
