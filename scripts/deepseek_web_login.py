#!/usr/bin/env python3
"""Refresh a managed DeepSeek web token through a real browser login.

This script is intentionally separate from the Go service. DeepSeek's web login
can be fronted by AWS WAF browser challenges, so the supported bootstrap path is
to run Chromium with Playwright, let the page solve the browser challenge, then
persist the returned DeepSeek token into config.json.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Any

from playwright.async_api import Error as PlaywrightError
from playwright.async_api import TimeoutError as PlaywrightTimeoutError
from playwright.async_api import async_playwright


LOGIN_URL = "https://chat.deepseek.com/sign_in"
CURRENT_USER_URL = "https://chat.deepseek.com/api/v0/users/current"
LOGIN_PATH = "/api/v0/users/login"


def load_config(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def save_config(path: Path, cfg: dict[str, Any]) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
        f.write("\n")
    tmp.replace(path)


def account_identifier(account: dict[str, Any]) -> str:
    return (str(account.get("email") or "").strip() or str(account.get("mobile") or "").strip())


def select_account(cfg: dict[str, Any], index: int) -> dict[str, Any]:
    accounts = cfg.get("accounts")
    if not isinstance(accounts, list) or not accounts:
        raise RuntimeError("config has no accounts")
    if index < 0 or index >= len(accounts):
        raise RuntimeError(f"account index {index} out of range")
    account = accounts[index]
    if not isinstance(account, dict):
        raise RuntimeError(f"account index {index} is not an object")
    return account


async def fill_first_visible(page: Any, selectors: list[str], value: str, label: str) -> None:
    last_error: Exception | None = None
    for selector in selectors:
        locator = page.locator(selector).first
        try:
            await locator.wait_for(state="visible", timeout=2500)
            await locator.fill(value)
            return
        except Exception as exc:  # Playwright has multiple selector failure types.
            last_error = exc
    raise RuntimeError(f"could not find visible {label} input") from last_error


async def click_first_visible(page: Any, selectors: list[str], label: str) -> bool:
    for selector in selectors:
        locator = page.locator(selector).first
        try:
            await locator.wait_for(state="visible", timeout=1800)
            await locator.click()
            return True
        except Exception:
            continue
    return False


async def read_current_user(page: Any) -> dict[str, Any]:
    return await page.evaluate(
        """async (url) => {
            const response = await fetch(url, {
                credentials: 'include',
                headers: { 'accept': 'application/json' },
            });
            const text = await response.text();
            let body = {};
            try { body = text ? JSON.parse(text) : {}; } catch (_) {}
            return { status: response.status, body };
        }""",
        CURRENT_USER_URL,
    )


def extract_token(obj: dict[str, Any]) -> str:
    data = obj.get("data") if isinstance(obj, dict) else None
    data = data if isinstance(data, dict) else {}
    biz_data = data.get("biz_data")
    biz_data = biz_data if isinstance(biz_data, dict) else {}
    user = biz_data.get("user")
    user = user if isinstance(user, dict) else {}
    token = str(user.get("token") or biz_data.get("token") or "").strip()
    return token


async def run(args: argparse.Namespace) -> int:
    cfg_path = Path(args.config).resolve()
    cfg = load_config(cfg_path)
    account = select_account(cfg, args.account_index)

    email = os.environ.get("DEEPSEEK_EMAIL", "").strip() or str(account.get("email") or "").strip()
    password = os.environ.get("DEEPSEEK_PASSWORD", "").strip() or str(account.get("password") or "").strip()
    if not email:
        raise RuntimeError("account email is empty")
    if not password:
        raise RuntimeError("account password is empty")

    seen: dict[str, str] = {
        "token": "",
        "device_id": str(account.get("device_id") or "").strip(),
        "login_device_id": str(account.get("login_device_id") or "").strip(),
    }
    login_statuses: list[int] = []

    async with async_playwright() as p:
        browser = await p.chromium.launch(
            headless=not args.headed,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--disable-dev-shm-usage",
            ],
        )
        context = await browser.new_context(
            locale=args.locale,
            timezone_id=args.timezone,
            user_agent=args.user_agent,
            viewport={"width": 1280, "height": 900},
        )
        page = await context.new_page()

        async def on_request(request: Any) -> None:
            if LOGIN_PATH not in request.url or request.method.upper() != "POST":
                return
            headers = await request.all_headers()
            x_device_id = headers.get("x-device-id", "").strip()
            if x_device_id:
                seen["device_id"] = x_device_id
            try:
                body = json.loads(request.post_data or "{}")
            except json.JSONDecodeError:
                return
            login_device_id = str(body.get("device_id") or "").strip()
            if login_device_id:
                seen["login_device_id"] = login_device_id

        async def on_response(response: Any) -> None:
            if LOGIN_PATH not in response.url:
                return
            login_statuses.append(response.status)
            if response.status != 200:
                return
            try:
                body = await response.json()
            except PlaywrightError:
                return
            token = extract_token(body)
            if token:
                seen["token"] = token

        page.on("request", on_request)
        page.on("response", on_response)

        await page.goto(LOGIN_URL, wait_until="domcontentloaded", timeout=args.timeout_ms)
        await page.wait_for_load_state("networkidle", timeout=args.timeout_ms)

        await fill_first_visible(
            page,
            [
                "input[type='email']",
                "input[name='email']",
                "input[autocomplete='username']",
                "input[placeholder*='email' i]",
                "input[type='text']",
            ],
            email,
            "email",
        )
        await fill_first_visible(
            page,
            [
                "input[type='password']",
                "input[name='password']",
                "input[autocomplete='current-password']",
                "input[placeholder*='password' i]",
            ],
            password,
            "password",
        )

        clicked = await click_first_visible(
            page,
            [
                "button[type='submit']",
                "button:has-text('Log in')",
                "button:has-text('Sign in')",
                "button:has-text('登录')",
            ],
            "submit",
        )
        if not clicked:
            await page.keyboard.press("Enter")

        deadline = asyncio.get_running_loop().time() + (args.timeout_ms / 1000)
        current_status = 0
        while asyncio.get_running_loop().time() < deadline:
            if seen["token"]:
                break
            await page.wait_for_timeout(1000)
            try:
                current = await read_current_user(page)
            except PlaywrightError:
                continue
            current_status = int(current.get("status") or 0)
            token = extract_token(current.get("body") or {})
            if token:
                seen["token"] = token
                break

        await browser.close()

    if not seen["token"]:
        statuses = ",".join(str(s) for s in login_statuses) or "-"
        raise RuntimeError(
            "browser login did not yield a token "
            f"(login_statuses={statuses}, current_status={current_status}, "
            f"device_id_len={len(seen['device_id'])}, login_device_id_len={len(seen['login_device_id'])})"
        )
    if not seen["device_id"]:
        raise RuntimeError("login succeeded but x-device-id was not captured")
    if not seen["login_device_id"]:
        raise RuntimeError("login succeeded but login device_id was not captured")

    account["token"] = seen["token"]
    account["device_id"] = seen["device_id"]
    account["login_device_id"] = seen["login_device_id"]
    save_config(cfg_path, cfg)

    print(
        "updated account",
        args.account_index,
        account_identifier(account),
        "token_len",
        len(seen["token"]),
        "device_id_len",
        len(seen["device_id"]),
        "login_device_id_len",
        len(seen["login_device_id"]),
    )
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Refresh DeepSeek web token using Playwright Chromium")
    parser.add_argument("--config", default="config.json", help="Path to config.json")
    parser.add_argument("--account-index", type=int, default=0, help="accounts[] index to refresh")
    parser.add_argument("--headed", action="store_true", help="Run browser headed instead of headless")
    parser.add_argument("--timeout-ms", type=int, default=120_000, help="Overall login wait timeout")
    parser.add_argument("--locale", default="en-US", help="Browser locale")
    parser.add_argument("--timezone", default="Europe/Moscow", help="Browser timezone")
    parser.add_argument(
        "--user-agent",
        default=(
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "AppleWebKit/537.36 (KHTML, like Gecko) "
            "Chrome/154.0.0.0 Safari/537.36"
        ),
        help="Browser user agent",
    )
    return parser.parse_args()


def main() -> int:
    try:
        return asyncio.run(run(parse_args()))
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
