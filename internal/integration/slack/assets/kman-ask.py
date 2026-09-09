#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

PROTOCOL_VERSION = "2025-06-18"
SERVER_INFO = {"name": "kman-ask", "version": "1.0.0"}
API = "https://slack.com/api"

POLL_INTERVAL_SECONDS = 3
DEFAULT_TIMEOUT_SECONDS = 600


def bot_token() -> str:
    value = os.environ.get("SLACK_BOT_TOKEN")
    if not value:
        raise RuntimeError("SLACK_BOT_TOKEN is not set in the environment")
    return value


def channel() -> str:
    value = os.environ.get("KMAN_SLACK_CHANNEL")
    if not value:
        raise RuntimeError("KMAN_SLACK_CHANNEL is not set in the environment")
    return value


def request(method: str, path: str, body: dict) -> dict:
    params = {k: v for k, v in body.items() if v is not None}
    data = urllib.parse.urlencode(params).encode()
    headers = {
        "Authorization": f"Bearer {bot_token()}",
        "Content-Type": "application/x-www-form-urlencoded",
    }
    req = urllib.request.Request(f"{API}/{path}", data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as resp:
        payload = json.loads(resp.read())
    if not payload.get("ok"):
        raise RuntimeError(f"slack {path} failed: {payload.get('error')}")
    return payload


def format_question(question: str, header: str, options: list[str]) -> str:
    lines = [f"*{header}*", question] if header else [question]
    if options:
        lines.append("")
        lines.extend(f"{i + 1}. {opt}" for i, opt in enumerate(options))
        lines.append("")
        lines.append("Reply in this thread with your answer (a number is fine).")
    return "\n".join(lines)


def tool_ask_user_question(arguments: dict) -> str:
    question = arguments.get("question") or ""
    if not question:
        raise RuntimeError("question is required")
    header = arguments.get("header") or ""
    options = arguments.get("options") or []
    timeout = int(arguments.get("timeout_seconds") or DEFAULT_TIMEOUT_SECONDS)

    thread_ts = os.environ.get("KMAN_SLACK_THREAD_TS") or ""
    posted = request("POST", "chat.postMessage", {
        "channel": channel(),
        "text": format_question(question, header, options),
        "thread_ts": thread_ts or None,
    })
    anchor = thread_ts or posted.get("ts")

    deadline = time.time() + timeout
    seen_ts = posted.get("ts")
    while time.time() < deadline:
        time.sleep(POLL_INTERVAL_SECONDS)
        replies = request("POST", "conversations.replies", {"channel": channel(), "ts": anchor})
        for msg in replies.get("messages", []):
            if msg.get("bot_id"):
                continue
            if msg.get("ts") in (seen_ts, anchor):
                continue
            if float(msg.get("ts", "0")) <= float(seen_ts or "0"):
                continue
            return msg.get("text", "")
    raise RuntimeError(f"no reply within {timeout}s")


TOOLS = [
    {
        "name": "ask_user_question",
        "description": "Ask a human a question over Slack and block until they answer. Use this instead of the "
                        "built-in AskUserQuestion tool, which is disabled for this run.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "question": {"type": "string", "description": "The question to ask."},
                "header": {"type": "string", "description": "Short label shown above the question."},
                "options": {"type": "array", "items": {"type": "string"}, "description": "Suggested answers, shown as a numbered list."},
                "timeout_seconds": {"type": "integer", "description": f"How long to wait for a reply, default {DEFAULT_TIMEOUT_SECONDS}."},
            },
            "required": ["question"],
        },
        "handler": tool_ask_user_question,
    },
]

HANDLERS = {tool["name"]: tool["handler"] for tool in TOOLS}
TOOL_SPECS = [{k: v for k, v in tool.items() if k != "handler"} for tool in TOOLS]


def respond(message_id, result=None, error=None) -> None:
    message = {"jsonrpc": "2.0", "id": message_id}
    if error is not None:
        message["error"] = error
    else:
        message["result"] = result
    sys.stdout.write(json.dumps(message) + "\n")
    sys.stdout.flush()


def handle(message: dict) -> None:
    method = message.get("method")
    message_id = message.get("id")

    if method == "initialize":
        requested = (message.get("params") or {}).get("protocolVersion") or PROTOCOL_VERSION
        respond(message_id, {
            "protocolVersion": requested,
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": SERVER_INFO,
        })
        return

    if method in ("notifications/initialized", "notifications/cancelled"):
        return

    if method == "ping":
        respond(message_id, {})
        return

    if method == "tools/list":
        respond(message_id, {"tools": TOOL_SPECS})
        return

    if method == "tools/call":
        params = message.get("params") or {}
        name = params.get("name")
        handler = HANDLERS.get(name)
        if handler is None:
            respond(message_id, error={"code": -32602, "message": f"unknown tool {name}"})
            return
        try:
            text = handler(params.get("arguments") or {})
            respond(message_id, {"content": [{"type": "text", "text": text}], "isError": False})
        except Exception as exc:
            respond(message_id, {"content": [{"type": "text", "text": str(exc)}], "isError": True})
        return

    if message_id is not None:
        respond(message_id, error={"code": -32601, "message": f"unknown method {method}"})


def main() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            message = json.loads(line)
        except json.JSONDecodeError:
            continue
        handle(message)
    return 0


if __name__ == "__main__":
    sys.exit(main())
