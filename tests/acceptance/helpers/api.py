import requests

BACKEND_URL = "http://localhost:5000"


class APIClient:
    def __init__(self, base_url=BACKEND_URL):
        self.base_url = base_url
        self.session = requests.Session()

    def _request(self, method, path, **kwargs):
        url = f"{self.base_url}{path}"
        kwargs.setdefault("timeout", 30)
        r = self.session.request(method, url, **kwargs)
        if r.status_code >= 400:
            raise Exception(f"API error {r.status_code}: {r.text}")
        return r.json()

    def get(self, path, **kwargs):
        return self._request("GET", path, **kwargs)

    def post(self, path, **kwargs):
        return self._request("POST", path, **kwargs)

    def put(self, path, **kwargs):
        return self._request("PUT", path, **kwargs)

    def delete(self, path, **kwargs):
        return self._request("DELETE", path, **kwargs)
