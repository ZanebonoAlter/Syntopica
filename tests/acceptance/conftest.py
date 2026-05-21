import pytest
import requests

BACKEND_URL = "http://localhost:5000"
FRONTEND_URL = "http://localhost:3000"


@pytest.fixture(scope="session", autouse=True)
def check_environment():
    try:
        requests.get(f"{BACKEND_URL}/api/feeds", timeout=5).raise_for_status()
    except Exception:
        pytest.exit("后端未运行在 localhost:5000")

    try:
        requests.get(FRONTEND_URL, timeout=5).raise_for_status()
    except Exception:
        pytest.exit("前端未运行在 localhost:3000")
