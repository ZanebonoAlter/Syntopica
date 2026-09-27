"""energy_data MCP server (stdio).

Thin FastMCP wrapper around sources.py. Official mcp SDK only — no hand-rolled
JSON-RPC. stdout carries MCP protocol messages exclusively; logs go to stderr.

Tools (fixed read-only, no URL/path/code parameters):
- eia_wpsr_table1(section)      US weekly petroleum status report table 1
- jodi_oil_primary(...)         JODI Oil Primary annual CSV (product=CRUDEOIL)
"""

from __future__ import annotations

import functools
import logging
import sys
from datetime import datetime, timezone
from typing import Any, Literal

import httpx

try:
    from mcp.server.mcpserver import MCPServer
except ImportError as exc:  # pragma: no cover - older SDK layout (mcp 1.x)
    try:
        from mcp.server.fastmcp import FastMCP as MCPServer  # type: ignore[assignment]
    except ImportError:
        raise SystemExit(
            f"[SOURCE_UNAVAILABLE] mcp SDK not importable: {exc}; run 'uv sync' first"
        ) from exc

try:
    from mcp.server.mcpserver.exceptions import ToolError
except ImportError:  # mcp 1.x layout  # pragma: no cover
    from mcp.server.fastmcp.exceptions import ToolError  # type: ignore[no-redef]

import sources
from sources import Fetcher, SourceError

logging.basicConfig(
    level=logging.INFO,
    stream=sys.stderr,
    format="%(asctime)s %(levelname)s energy-mcp %(name)s: %(message)s",
)
logger = logging.getLogger("server")

mcp = MCPServer("energy_data")
fetcher = Fetcher()


def _tool_error(err: SourceError) -> ToolError:
    return ToolError(f"[{err.code}] {err.message}")


def _http_guard(coro):
    """Wrap a tool coroutine: httpx transport failures -> SOURCE_UNAVAILABLE,
    decode failures -> SCHEMA_CHANGED (stable codes, nothing swallowed).
    functools.wraps keeps name/doc/annotations and (via __wrapped__)
    the signature incl. defaults for schema generation.
    """

    async def wrapped(*args, **kwargs):
        try:
            return await coro(*args, **kwargs)
        except ToolError:
            raise
        except SourceError as err:
            raise _tool_error(err) from err
        except UnicodeDecodeError as err:
            raise _tool_error(
                SourceError("SCHEMA_CHANGED", f"source bytes failed expected decoding: {err}")
            ) from err
        except (httpx.HTTPError, OSError) as err:
            raise _tool_error(
                SourceError("SOURCE_UNAVAILABLE", f"network/transport failure: {err}")
            ) from err

    return functools.wraps(coro)(wrapped)


async def _get_eia() -> sources.FetchedDoc:
    doc = await fetcher.fetch(sources.EIA_WPSR_TABLE1_URL)
    if doc.status != 200:
        raise SourceError(
            "SOURCE_UNAVAILABLE", f"EIA table1.csv HTTP status {doc.status}"
        )
    if sources._looks_like_html(doc.body):
        raise SourceError(
            "SOURCE_UNAVAILABLE", "EIA table1.csv response body is HTML, not CSV"
        )
    return doc


async def _get_jodi(year: int) -> sources.FetchedDoc:
    url = sources.JODI_PRIMARY_URL_TEMPLATE.format(year=year)
    doc = await fetcher.fetch(url)
    if doc.status == 404:
        raise LookupError(url)
    if doc.status != 200:
        raise SourceError(
            "SOURCE_UNAVAILABLE", f"JODI {year} file HTTP status {doc.status}"
        )
    if sources._looks_like_html(doc.body):
        raise SourceError(
            "SOURCE_UNAVAILABLE", f"JODI {year} response body is HTML, not CSV"
        )
    return doc


@mcp.tool()
@_http_guard
async def eia_wpsr_table1(
    section: Literal["stocks", "supply"] = "stocks",
) -> dict[str, Any]:
    """US weekly petroleum status report (EIA WPSR) Table 1, crude oil only.

    section="stocks": US crude oil stocks — total (incl SPR), commercial
    (excluding SPR), and SPR — in million barrels (MMbbl).
    section="supply": US crude oil domestic production, imports and exports in
    thousand barrels per day (Mb/d).

    Returns current-week and prior-week values with each week-ending date,
    original row labels and source row numbers. US data only; weekly; no
    forecasts, no cross-source totals.
    """
    if section not in ("stocks", "supply"):
        raise _tool_error(
            SourceError("INVALID_ARGUMENT", f"section must be stocks|supply, got {section!r}")
        )
    try:
        doc = await _get_eia()
        text = doc.body.decode("cp1252")
        return sources.eia_result(section, text, doc)
    except SourceError as err:
        if err.code == "SCHEMA_CHANGED":
            # source file parsed with a stale/malformed body: evict so the
            # next call re-requests instead of replaying 15 minutes of bad parse
            fetcher.invalidate(sources.EIA_WPSR_TABLE1_URL)
        raise
    except UnicodeDecodeError as err:
        fetcher.invalidate(sources.EIA_WPSR_TABLE1_URL)
        raise


@mcp.tool()
@_http_guard
async def jodi_oil_primary(
    geo: str = "US",
    flow: Literal["production", "imports", "exports", "closing_stocks"] = "production",
    unit: Literal["KBD", "KBBL"] | None = None,
    month: str | None = None,
) -> dict[str, Any]:
    """JODI Oil Primary world database — crude oil (CRUDEOIL) only, monthly.

    geo: two-uppercase-letter economy code present in the data (e.g. "US").
    flow: production|imports|exports|closing_stocks.
    unit: "KBD" (thousand barrels per day, default for flows), "KBBL" (thousand
    barrels, default for closing_stocks) or null for the flow default. No unit
    conversion; closing_stocks+KBD is rejected.
    month: "YYYY-MM" (2002..current year, not in the future) or null for the
    latest month of the current-year file (falling back to the previous year's
    file at most once when the current year is 404 — noted in the result).

    Missing markers ('-', '..', 'x') return null values with raw_value kept.
    assessment_code is passed through uninterpreted.
    """
    try:
        flow_code, resolved_unit = sources.validate_jodi_args(geo, flow, unit, month)
    except SourceError as err:
        raise _tool_error(err) from err

    now = datetime.now(timezone.utc)
    try:
        if month is not None:
            year_used = int(month[:4])
            try:
                doc = await _get_jodi(year_used)
            except LookupError as err:
                raise _tool_error(
                    SourceError(
                        "SOURCE_UNAVAILABLE",
                        f"JODI annual file for {year_used} (explicit month {month})"
                        f" not found: {err.args[0]}",
                    )
                ) from err
            year_strategy = "explicit month -> matching annual file"
        else:
            year_used = now.year
            try:
                doc = await _get_jodi(year_used)
                year_strategy = "latest month of current-year file"
            except LookupError:
                year_used = now.year - 1
                doc = await _get_jodi(year_used)
                year_strategy = (
                    "current-year file 404 -> previous-year file (single fallback)"
                )
    except ToolError:
        raise
    except SourceError as err:
        raise _tool_error(err) from err
    except LookupError:
        raise _tool_error(
            SourceError(
                "SOURCE_UNAVAILABLE",
                f"JODI annual files not available (tried {now.year} and {now.year - 1})",
            )
        ) from None

    try:
        rows = sources.parse_jodi_primary(doc.body.decode("utf-8"))
        return sources.jodi_result(
            doc, rows, geo, flow, resolved_unit, month, year_used, year_strategy
        )
    except SourceError as err:
        if err.code == "SCHEMA_CHANGED":
            # evict the request key actually used (doc.url may differ after a
            # same-host signature redirect, but the cache key is the request URL)
            fetcher.invalidate(sources.JODI_PRIMARY_URL_TEMPLATE.format(year=year_used))
        raise
    except UnicodeDecodeError as err:
        fetcher.invalidate(sources.JODI_PRIMARY_URL_TEMPLATE.format(year=year_used))
        raise


if __name__ == "__main__":
    logger.info("energy_data MCP server starting (stdio)")
    mcp.run()
