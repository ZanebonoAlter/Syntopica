"""Sources layer for the energy_data MCP server.

Fixed official HTTP fetch + pure CSV parsing for:
- EIA Weekly Petroleum Status Report table 1 (https://ir.eia.gov/wpsr/table1.csv)
- JODI Oil Primary world database annual CSV (jodidata.org annual template)

Red lines (spec: dsh-energy-mcp-tools):
- Only the two fixed official HTTPS hosts are ever requested; tools expose no
  URL/path/code parameters.
- Missing markers map to null (never 0); units are never auto-converted or
  merged across sources/frequencies.
- Errors carry stable codes: INVALID_ARGUMENT / SOURCE_UNAVAILABLE / SCHEMA_CHANGED.
"""

from __future__ import annotations

import csv
import hashlib
import io
import math
import re
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal, InvalidOperation
from email.utils import parsedate_to_datetime
from typing import Any
from urllib.parse import urljoin, urlparse

import httpx

# ── fixed sources ────────────────────────────────────────────────────────────

EIA_WPSR_TABLE1_URL = "https://ir.eia.gov/wpsr/table1.csv"
JODI_PRIMARY_URL_TEMPLATE = (
    "https://www.jodidata.org/_resources/files/downloads/oil-data/"
    "annual-csv/primary/primaryyear{year}.csv"
)
ALLOWED_HOSTS = {"ir.eia.gov", "www.jodidata.org"}

JODI_MIN_YEAR = 2002

# resource guardrails
MAX_RESPONSE_BYTES = 32 * 1024 * 1024  # 32 MB per response
HTTP_TIMEOUT_SECONDS = 45.0            # per-request timeout; tool-level budget
MAX_REDIRECTS = 3
CACHE_TTL_SECONDS = 900.0              # 15 minutes, in-memory, success only

USER_AGENT = "syntopica-energy-mcp/0.1 (read-only public data)"

# ── errors ───────────────────────────────────────────────────────────────────


class SourceError(Exception):
    """Stable error code + human-readable message (no exception swallowing)."""

    def __init__(self, code: str, message: str):
        super().__init__(f"[{code}] {message}")
        self.code = code
        self.message = message


# ── HTTP fetch layer (shared) ────────────────────────────────────────────────


@dataclass
class RawResponse:
    status: int
    headers: dict[str, str]
    body: bytes


@dataclass
class FetchedDoc:
    url: str  # final URL after bounded same-host redirects
    status: int
    body: bytes
    retrieved_at: str
    last_modified: str | None
    sha256: str
    from_cache: bool = False


class Fetcher:
    """Bounded same-host HTTPS fetcher with in-memory TTL cache.

    Only successful (2xx) documents are cached; errors are never cached and a
    cached hit keeps its original retrieved_at/sha256 (never refreshed to fake
    a newer time).
    """

    def __init__(
        self,
        ttl: float = CACHE_TTL_SECONDS,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        self._ttl = ttl
        self._transport = transport  # unit tests inject MockTransport; None = real network
        self._cache: dict[str, tuple[float, FetchedDoc]] = {}

    async def fetch(self, url: str) -> FetchedDoc:
        hit = self._cache.get(url)
        if hit is not None and hit[0] > time.monotonic():
            cached = hit[1]
            return FetchedDoc(
                url=cached.url,
                status=cached.status,
                body=cached.body,
                retrieved_at=cached.retrieved_at,
                last_modified=cached.last_modified,
                sha256=cached.sha256,
                from_cache=True,
            )

        current = url
        final_status = 0
        final_headers: dict[str, str] = {}
        final_body = b""
        for _ in range(MAX_REDIRECTS + 1):
            if urlparse(current).hostname not in ALLOWED_HOSTS:
                raise SourceError(
                    "SOURCE_UNAVAILABLE", f"host not allowed: {urlparse(current).hostname!r}"
                )
            raw = await self._single_request(current)
            if raw.status in (301, 302, 303, 307, 308):
                location = raw.headers.get("location")
                if not location:
                    raise SourceError(
                        "SOURCE_UNAVAILABLE", f"redirect without Location from {current}"
                    )
                target = urljoin(current, location)
                if urlparse(target).hostname != urlparse(current).hostname:
                    raise SourceError(
                        "SOURCE_UNAVAILABLE",
                        f"cross-host redirect blocked: {current} -> {target}",
                    )
                current = target
                continue
            final_status, final_headers, final_body = raw.status, raw.headers, raw.body
            break
        else:
            raise SourceError("SOURCE_UNAVAILABLE", f"too many redirects for {url}")

        content_length = final_headers.get("content-length")
        if content_length is not None:
            try:
                if int(content_length) > MAX_RESPONSE_BYTES:
                    raise SourceError(
                        "SOURCE_UNAVAILABLE",
                        f"response too large: {content_length} bytes > {MAX_RESPONSE_BYTES}",
                    )
            except ValueError:
                pass
        if len(final_body) > MAX_RESPONSE_BYTES:
            raise SourceError(
                "SOURCE_UNAVAILABLE", f"response too large: {len(final_body)} bytes"
            )

        doc = FetchedDoc(
            url=current,
            status=final_status,
            body=final_body,
            retrieved_at=datetime.now(timezone.utc).isoformat(),
            last_modified=_normalize_http_date(final_headers.get("last-modified")),
            sha256=hashlib.sha256(final_body).hexdigest(),
        )
        # Success-only cache, and HTML error pages never enter it: a 2xx HTML
        # body must be re-requested next call (server raises SOURCE_UNAVAILABLE
        # on it; caching it would replay a 15-minute-stale error page).
        if 200 <= final_status < 300 and not _looks_like_html(final_body):
            self._cache[url] = (time.monotonic() + self._ttl, doc)
        return doc

    async def _single_request(self, url: str) -> RawResponse:
        """Single HTTP GET with bounded incremental streaming.

        Body is read via aiter_bytes() and aborted as soon as MAX_RESPONSE_BYTES
        is exceeded, so a chunked/unbounded response is never fully buffered.
        Content-Length may pre-reject but is never relied on. Each hop must be
        https on the default port 443 (no http downgrade via redirects).
        Unit tests patch this method or inject a MockTransport via
        self._transport; they never touch the real network.
        """
        parsed = urlparse(url)
        if parsed.scheme != "https" or parsed.port not in (None, 443):
            raise SourceError(
                "SOURCE_UNAVAILABLE",
                f"only https on default port 443 is allowed (got {url});"
                " no http downgrade",
            )
        try:
            async with httpx.AsyncClient(
                follow_redirects=False,
                timeout=httpx.Timeout(HTTP_TIMEOUT_SECONDS),
                headers={"User-Agent": USER_AGENT},
                transport=self._transport,
            ) as client:
                async with client.stream("GET", url) as resp:
                    status = resp.status_code
                    headers = {k.lower(): v for k, v in resp.headers.items()}
                    content_length = headers.get("content-length")
                    if content_length is not None:
                        try:
                            if int(content_length) > MAX_RESPONSE_BYTES:
                                raise SourceError(
                                    "SOURCE_UNAVAILABLE",
                                    f"response too large: Content-Length"
                                    f" {content_length} > {MAX_RESPONSE_BYTES} bytes",
                                )
                        except ValueError:
                            pass  # unparseable header: rely on streaming bound
                    body = bytearray()
                    async for chunk in resp.aiter_bytes():
                        body.extend(chunk)
                        if len(body) > MAX_RESPONSE_BYTES:
                            # raising closes the response; no further chunks read
                            raise SourceError(
                                "SOURCE_UNAVAILABLE",
                                f"response too large: > {MAX_RESPONSE_BYTES}"
                                " bytes streamed",
                            )
        except SourceError:
            raise
        except (httpx.HTTPError, OSError) as err:
            raise SourceError(
                "SOURCE_UNAVAILABLE", f"network/transport failure: {err}"
            ) from err
        return RawResponse(status, headers, bytes(body))

    def invalidate(self, url: str) -> None:
        """Evict one request key (bad decoded/parsed doc must be re-fetched)."""
        self._cache.pop(url, None)

    def clear_cache(self) -> None:
        self._cache.clear()


def _normalize_http_date(value: str | None) -> str | None:
    if not value:
        return None
    try:
        return parsedate_to_datetime(value).astimezone(timezone.utc).isoformat()
    except (TypeError, ValueError):
        return value


def _looks_like_html(body: bytes) -> bool:
    head = body.lstrip()[:32].lower()
    return head.startswith(b"<!doctype") or head.startswith(b"<html") or head.startswith(b"<")


def _utc_now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


# ── number / label helpers ───────────────────────────────────────────────────

EN_DASH = "–"  # CP1252 0x96


def _decimal_to_float(cell: str) -> float | None:
    """Parse a numeric CSV cell into a finite float.

    Rejects NaN/Infinity decimals and values whose float conversion overflows
    to inf (e.g. 1e999); returns None in those cases so the caller can decide
    missing-marker vs SCHEMA_CHANGED. A real "0" stays 0.0; negatives and
    thousand-separated values parse normally.
    """
    try:
        dec = Decimal(cell.replace(",", ""))
    except InvalidOperation:
        return None
    if not dec.is_finite():
        return None  # NaN / Infinity / -Infinity
    try:
        value = float(dec)
    except OverflowError:
        return None  # huge magnitude decimal -> inf
    if not math.isfinite(value):
        return None
    return value


def parse_number(cell: str | None) -> tuple[float | None, str | None]:
    """Parse a numeric CSV cell after quote/comma handling.

    Returns (value, missing_reason). En-dash pairs, empty cells -> (None, reason).
    A real "0" stays 0.0. Values that are neither numeric nor a known missing
    marker return (None, None) and raise SCHEMA_CHANGED via the caller
    (see _record_eia_row / _jodi_cell). NaN/inf/overflow are rejected the same
    way — never returned as values.
    """
    raw = (cell or "").strip()
    if raw == "":
        return None, "empty cell"
    if raw.strip(EN_DASH).strip() == "":
        return None, "en-dash missing marker"
    value = _decimal_to_float(raw)
    if value is None:
        return None, None  # caller decides: marker vs schema drift
    return value, None


_FOOTNOTE_RE = re.compile(r"^\(\d+\)\s*")
_WS_RE = re.compile(r"\s+")


def normalize_label(raw: str) -> str:
    """Strip leading footnote number like '(1)     ' and collapse whitespace."""
    s = _FOOTNOTE_RE.sub("", raw or "")
    return _WS_RE.sub(" ", s).strip()


# ── EIA WPSR table1 ─────────────────────────────────────────────────────────

EIA_STOCKS_ROWS: list[tuple[str, str]] = [
    ("Crude Oil", "crude_stocks_total_incl_spr"),
    ("Commercial (Excluding SPR)", "crude_stocks_commercial"),
    ("Strategic Petroleum Reserve (SPR)", "crude_stocks_spr"),
]
EIA_SUPPLY_GROUP = "Crude Oil Supply"
EIA_SUPPLY_ROWS: list[tuple[str, str]] = [
    ("Domestic Production", "crude_production"),
    ("Imports", "crude_imports"),
    ("Exports", "crude_exports"),
]

_DATE_HEADER_RE = re.compile(r"^\d{1,2}/\d{1,2}/\d{2}$")


def _parse_mdy(token: str) -> str | None:
    """'8/28/26' -> '2026-08-28' (yy>=70 -> 1900s pivot)."""
    m = _DATE_HEADER_RE.match(token.strip())
    if not m:
        return None
    month, day, yy = (int(p) for p in token.strip().split("/"))
    year = 1900 + yy if yy >= 70 else 2000 + yy
    try:
        return datetime(year, month, day).date().isoformat()
    except ValueError:
        return None


@dataclass
class EiaRow:
    source_row: int
    label: str
    current_value: float | None
    current_raw: str | None
    prior_value: float | None
    prior_raw: str | None
    current_missing: str | None = None
    prior_missing: str | None = None


@dataclass
class EiaSection:
    rows: dict[str, EiaRow] = field(default_factory=dict)
    current_week_ending: str | None = None
    prior_week_ending: str | None = None


def parse_eia_table1(text: str) -> dict[str, EiaSection]:
    """Parse WPSR table1.csv into {section: {normalized_label: EiaRow}}.

    Section A (stocks): STUB_1 + weekly date columns (values in million barrels).
    Section B (supply): STUB_1 (group) + STUB_2 (row) + date columns (thousand
    barrels per day). Current week = first two date columns in each section.
    Any structural drift (missing date columns, missing required labels,
    duplicate conflicting rows) raises SCHEMA_CHANGED.
    """
    reader = csv.reader(io.StringIO(text, newline=""))
    sections: dict[str, EiaSection] = {"stocks": EiaSection(), "supply": EiaSection()}
    current_section: str | None = None
    date_cols: list[int] = []
    stocks_started = False
    supply_started = False

    try:
        for row_no, row in enumerate(reader, start=1):
            if not row or all(not c.strip() for c in row):
                continue
            first, second = (row + ["", ""])[:2]
            first, second = first.strip(), second.strip()
            if first == "STUB_1":
                # header row decides which section follows
                if second == "STUB_2":
                    if supply_started:
                        raise SourceError(
                            "SCHEMA_CHANGED", f"row {row_no}: duplicate supply section header"
                        )
                    current_section = "supply"
                    supply_started = True
                elif _DATE_HEADER_RE.match(second):
                    if stocks_started:
                        raise SourceError(
                            "SCHEMA_CHANGED", f"row {row_no}: duplicate stocks section header"
                        )
                    current_section = "stocks"
                    stocks_started = True
                else:
                    raise SourceError(
                        "SCHEMA_CHANGED",
                        f"row {row_no}: unrecognized section header second cell {second!r}",
                    )
                date_cols = [i for i, c in enumerate(row) if _DATE_HEADER_RE.match(c.strip())]
                if len(date_cols) < 2:
                    raise SourceError(
                        "SCHEMA_CHANGED",
                        f"row {row_no}: fewer than 2 date columns ({len(date_cols)})",
                    )
                sections[current_section].current_week_ending = _parse_mdy(
                    row[date_cols[0]].strip()
                )
                sections[current_section].prior_week_ending = _parse_mdy(
                    row[date_cols[1]].strip()
                )
                continue
            if current_section is None:
                continue  # preamble before first header (none observed, tolerated)

            if current_section == "stocks":
                label = normalize_label(first)
                if not label:
                    continue
                cur_idx, prior_idx = date_cols[0], date_cols[1]
                _record_eia_row(
                    sections["stocks"].rows,
                    row_no,
                    label,
                    row[cur_idx] if cur_idx < len(row) else "",
                    row[prior_idx] if prior_idx < len(row) else "",
                )
            else:
                group = normalize_label(first)
                label = normalize_label(second)
                if group == EIA_SUPPLY_GROUP and label:
                    cur_idx, prior_idx = date_cols[0], date_cols[1]
                    _record_eia_row(
                        sections["supply"].rows,
                        row_no,
                        label,
                        row[cur_idx] if cur_idx < len(row) else "",
                        row[prior_idx] if prior_idx < len(row) else "",
                    )
    except csv.Error as err:
        # csv.reader raises for e.g. fields beyond the field_size_limit;
        # surface as stable SCHEMA_CHANGED, never as a raw crash
        raise SourceError(
            "SCHEMA_CHANGED", f"CSV parse failure in EIA table1: {err}"
        ) from err

    missing = [lbl for lbl, _ in EIA_STOCKS_ROWS if lbl not in sections["stocks"].rows]
    missing += [lbl for lbl, _ in EIA_SUPPLY_ROWS if lbl not in sections["supply"].rows]
    if missing:
        raise SourceError("SCHEMA_CHANGED", f"required labels missing: {missing}")
    if not sections["stocks"].current_week_ending or not sections["supply"].current_week_ending:
        raise SourceError("SCHEMA_CHANGED", "week-ending dates failed to parse")
    return sections


def _record_eia_row(
    store: dict[str, EiaRow],
    row_no: int,
    label: str,
    cur_cell: str,
    prior_cell: str,
) -> None:
    cur_val, cur_missing = parse_number(cur_cell)
    prior_val, prior_missing = parse_number(prior_cell)
    if cur_val is None and cur_missing is None:
        raise SourceError(
            "SCHEMA_CHANGED",
            f"row {row_no} {label!r}: current cell {cur_cell!r} is neither"
            " numeric nor a known missing marker",
        )
    if prior_val is None and prior_missing is None:
        raise SourceError(
            "SCHEMA_CHANGED",
            f"row {row_no} {label!r}: prior cell {prior_cell!r} is neither"
            " numeric nor a known missing marker",
        )
    entry = EiaRow(
        source_row=row_no,
        label=label,
        current_value=cur_val,
        current_raw=cur_cell.strip(),
        current_missing=cur_missing,
        prior_value=prior_val,
        prior_raw=prior_cell.strip(),
        prior_missing=prior_missing,
    )
    existing = store.get(label)
    if existing is not None and _rows_conflict(existing, entry):
        raise SourceError(
            "SCHEMA_CHANGED",
            f"conflicting duplicate rows for label {label!r}: "
            f"line {existing.source_row} vs line {row_no}",
        )
    store[label] = entry


def _rows_conflict(a: EiaRow, b: EiaRow) -> bool:
    return (
        a.current_value != b.current_value
        or a.prior_value != b.prior_value
        or a.current_raw != b.current_raw
        or a.prior_raw != b.prior_raw
    )


def eia_result(section: str, text: str, meta: FetchedDoc) -> dict[str, Any]:
    sections = parse_eia_table1(text)
    if section == "stocks":
        targets, flow_unit = EIA_STOCKS_ROWS, "MMbbl"
        chosen = sections["stocks"]
    else:
        targets, flow_unit = EIA_SUPPLY_ROWS, "Mb/d"
        chosen = sections["supply"]

    observations = []
    for label, flow_id in targets:
        row = chosen.rows[label]
        obs = {
            "geo": "US",
            "product": "Crude Oil",
            "flow": flow_id,
            "label": row.label,
            "frequency": "weekly",
            "unit": flow_unit,
            "period": chosen.current_week_ending,
            "value": row.current_value,
            "raw_value": row.current_raw,
            "prior_period": chosen.prior_week_ending,
            "prior_value": row.prior_value,
            "prior_raw_value": row.prior_raw,
            "source_row": row.source_row,
        }
        if row.current_missing:
            obs["missing_reason"] = row.current_missing
        if row.prior_missing:
            obs["prior_missing_reason"] = row.prior_missing
        observations.append(obs)
    return {
        "source": "EIA Weekly Petroleum Status Report Table 1 (US only, weekly)",
        "url": meta.url,
        "retrieved_at": meta.retrieved_at,
        "last_modified": meta.last_modified,
        "source_sha256": meta.sha256,
        "from_cache": meta.from_cache,
        "notes": [
            "US data only; weekly period ending Friday per WPSR convention.",
            "Stocks in million barrels (MMbbl); supply flows in thousand barrels"
            " per day (Mb/d) per WPSR Table 1 definitions (units are not stated"
            " inside the CSV).",
            "No cross-source totals or unit conversions are performed.",
        ],
        "observations": observations,
    }


# ── JODI Oil Primary ────────────────────────────────────────────────────────

JODI_HEADER = [
    "REF_AREA",
    "TIME_PERIOD",
    "ENERGY_PRODUCT",
    "FLOW_BREAKDOWN",
    "UNIT_MEASURE",
    "OBS_VALUE",
    "ASSESSMENT_CODE",
]
JODI_FLOW_MAP = {
    "production": "INDPROD",
    "imports": "TOTIMPSB",
    "exports": "TOTEXPSB",
    "closing_stocks": "CLOSTLV",
}
JODI_DEFAULT_UNIT = {
    "production": "KBD",
    "imports": "KBD",
    "exports": "KBD",
    "closing_stocks": "KBBL",
}
JODI_MISSING_MARKERS = {"-", "..", "x"}
JODI_MONTH_RE = re.compile(r"^(\d{4})-(0[1-9]|1[0-2])$")
JODI_PRODUCT = "CRUDEOIL"


@dataclass
class JodiRow:
    source_row: int
    geo: str
    period: str
    product: str
    flow: str
    unit: str
    value: float | None
    raw_value: str
    missing_reason: str | None
    assessment_code: str | None


def parse_jodi_primary(text: str) -> list[JodiRow]:
    reader = csv.reader(io.StringIO(text, newline=""))
    row_no = 1
    try:
        header = next(reader)
        if [h.strip() for h in header] != JODI_HEADER:
            raise SourceError(
                "SCHEMA_CHANGED",
                f"unexpected JODI header: {[h.strip() for h in header]}",
            )
        rows: list[JodiRow] = []
        for row_no, row in enumerate(reader, start=2):
            if not row or all(not c.strip() for c in row):
                continue
            if len(row) != 7:
                raise SourceError(
                    "SCHEMA_CHANGED", f"row {row_no}: {len(row)} columns, expected 7"
                )
            geo, period, product, flow, unit, obs, code = (c.strip() for c in row)
            value, reason = _jodi_cell(obs)
            if value is None and reason is None:
                raise SourceError(
                    "SCHEMA_CHANGED",
                    f"row {row_no}: OBS_VALUE {obs!r} is neither numeric nor a known"
                    " missing marker",
                )
            rows.append(
                JodiRow(
                    source_row=row_no,
                    geo=geo,
                    period=period,
                    product=product,
                    flow=flow,
                    unit=unit,
                    value=value,
                    raw_value=obs,
                    missing_reason=reason,
                    assessment_code=code or None,
                )
            )
    except csv.Error as err:
        # csv.reader raises for e.g. fields beyond the field_size_limit;
        # surface as stable SCHEMA_CHANGED, never as a raw crash
        raise SourceError(
            "SCHEMA_CHANGED", f"CSV parse failure near row {row_no}: {err}"
        ) from err
    return rows


def _jodi_cell(obs: str) -> tuple[float | None, str | None]:
    if obs in JODI_MISSING_MARKERS:
        return None, f"missing marker {obs!r}"
    value = _decimal_to_float(obs)
    if value is None:
        return None, None  # caller decides: marker vs schema drift
    return value, None


def validate_jodi_args(geo: str, flow: str, unit: str | None, month: str | None) -> tuple[str, str]:
    """Validate pure-local arguments before any network call.

    Returns (flow_code, resolved_unit). Raises INVALID_ARGUMENT on bad input.
    """
    if flow not in JODI_FLOW_MAP:
        raise SourceError(
            "INVALID_ARGUMENT",
            f"flow must be one of {sorted(JODI_FLOW_MAP)}, got {flow!r}",
        )
    if not re.fullmatch(r"[A-Z]{2}", geo or ""):
        raise SourceError(
            "INVALID_ARGUMENT",
            f"geo must be exactly two uppercase letters (ISO-like code), got {geo!r}",
        )
    if unit is not None and unit not in ("KBD", "KBBL"):
        raise SourceError(
            "INVALID_ARGUMENT",
            f"unit must be KBD, KBBL or null (no auto conversion), got {unit!r}"
            " (CONVBBL is not used)",
        )
    resolved = unit or JODI_DEFAULT_UNIT[flow]
    if flow == "closing_stocks" and resolved == "KBD":
        raise SourceError(
            "INVALID_ARGUMENT",
            "closing_stocks with unit KBD is rejected (meaningless; stocks are KBBL;"
            " no conversion is performed)",
        )
    now = datetime.now(timezone.utc)
    if month is not None:
        m = JODI_MONTH_RE.match(month)
        if not m:
            raise SourceError(
                "INVALID_ARGUMENT", f"month must be YYYY-MM, got {month!r}"
            )
        year = int(m.group(1))
        if not (JODI_MIN_YEAR <= year <= now.year):
            raise SourceError(
                "INVALID_ARGUMENT",
                f"month year {year} out of supported range {JODI_MIN_YEAR}..{now.year}",
            )
        if year == now.year and month > f"{now.year:04d}-{now.month:02d}":
            raise SourceError(
                "INVALID_ARGUMENT",
                f"month {month} is in the future (current {now.year:04d}-{now.month:02d})",
            )
    return JODI_FLOW_MAP[flow], resolved


def jodi_result(
    doc: FetchedDoc,
    rows: list[JodiRow],
    geo: str,
    flow: str,
    unit: str,
    month: str | None,
    year_used: int,
    year_strategy: str,
) -> dict[str, Any]:
    flow_code = JODI_FLOW_MAP[flow]
    matched = [
        r
        for r in rows
        if r.geo == geo
        and r.product == JODI_PRODUCT
        and r.flow == flow_code
        and r.unit == unit
        and (month is None or r.period == month)
    ]
    selected: list[JodiRow] = []
    if month is not None:
        selected = matched
    elif matched:
        latest = max(r.period for r in matched)
        selected = [r for r in matched if r.period == latest]

    # A single OBS may legitimately appear once per unit; but a raw duplicate
    # (100/999 style re-publication) would silently yield two observations for
    # the same dimension+period. Group by the full dimension key: identical
    # records merge into one (rows stay traceable); any semantic conflict
    # (value / raw missing marker / assessment) is SCHEMA_CHANGED.
    groups: dict[tuple[str, str, str, str, str], list[JodiRow]] = {}
    for r in selected:
        key = (r.geo, r.product, r.flow, r.unit, r.period)
        groups.setdefault(key, []).append(r)

    observations = []
    for key, group in sorted(groups.items()):
        first = group[0]
        for other in group[1:]:
            if (
                other.value != first.value
                or other.raw_value != first.raw_value
                or other.missing_reason != first.missing_reason
                or other.assessment_code != first.assessment_code
            ):
                raise SourceError(
                    "SCHEMA_CHANGED",
                    f"conflicting duplicate rows for {key}: line"
                    f" {first.source_row} vs line {other.source_row}",
                )
        obs = {
            "geo": first.geo,
            "product": first.product,
            "flow": first.flow,
            "period": first.period,
            "frequency": "monthly",
            "unit": first.unit,
            "value": first.value,
            "raw_value": first.raw_value,
            "source_row": first.source_row,
            "assessment_code": first.assessment_code,
        }
        if first.missing_reason:
            obs["missing_reason"] = first.missing_reason
        if len(group) > 1:
            obs["source_rows"] = [r.source_row for r in group]
        observations.append(obs)

    return {
        "source": "JODI Oil Primary World Database (annual CSV snapshot)",
        "url": doc.url,
        "retrieved_at": doc.retrieved_at,
        "last_modified": doc.last_modified,
        "source_sha256": doc.sha256,
        "from_cache": doc.from_cache,
        "year_used": year_used,
        "year_strategy": year_strategy,
        "notes": [
            "TIME_PERIOD is the data month, not a publication date; annual files"
            " lag the data month by roughly 1.5-2 months and are full replacements"
            " (no revision history columns).",
            "assessment_code is passed through as-is; its official semantics are"
            " unverified (1/2/3), it is NOT a good/bad quality flag.",
            "Missing markers '-', '..', 'x' map to null and are kept in raw_value;"
            " values are never coerced to 0.",
            "Closing stocks coverage (SPR inclusion) is NOT assumed to match EIA"
            " commercial stocks; no cross-source comparison is performed.",
        ],
        "no_data": len(observations) == 0,
        "observations": observations,
    }
