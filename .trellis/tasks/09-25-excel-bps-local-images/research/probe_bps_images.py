"""Opt-in BPS protocol probe. Uses synthetic pixels and never prints credentials."""
import argparse
import base64
import binascii
import json
from pathlib import Path
import re
import struct
import subprocess
import time
import tomllib
import urllib.error
import urllib.request
import uuid
import zlib

ROOT = Path(__file__).resolve().parent


def database(sql):
    info = json.loads(subprocess.check_output(["docker", "inspect", "sub2api-postgres"], text=True))[0]
    env = dict(value.split("=", 1) for value in info["Config"]["Env"] if "=" in value)
    result = subprocess.run(["docker", "exec", "-i", "sub2api-postgres", "psql", "-U", env.get("POSTGRES_USER", "postgres"), "-d", env.get("POSTGRES_DB", "postgres"), "-At", "-f", "-"], input=sql, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError("database_query_failed")
    return [json.loads(line) for line in result.stdout.splitlines() if line.strip()]


def credentials():
    config = tomllib.loads((Path.home() / ".codex" / "config.toml").read_text(encoding="utf-8"))
    key = json.loads((Path.home() / ".codex" / "auth.json").read_text(encoding="utf-8"))["OPENAI_API_KEY"]
    escaped = key.replace("'", "''")
    rows = database("select row_to_json(t) from (select a.id,a.credentials,a.extra,a.proxy_id,u.upstream_model from usage_logs u join accounts a on a.id=u.account_id join api_keys k on k.id=u.api_key_id where k.key='" + escaped + "' and u.upstream_endpoint='/basispoints/api/responses' and a.status='active' and a.deleted_at is null order by u.id desc limit 1)t;")
    if not rows:
        raise RuntimeError("selected_bps_account_not_found")
    account = rows[0]
    if account["proxy_id"] is not None:
        raise RuntimeError("account_proxy_requires_matching_transport")
    if account["upstream_model"] != config["model"]:
        raise RuntimeError("mapped_model_requires_explicit_probe_match")
    if not account["extra"].get("openai_excel_bps"):
        raise RuntimeError("selected_account_bps_disabled")
    return account


def png_image():
    width = height = 128
    rows = []
    for y in range(height):
        row = bytearray([0])
        for x in range(width):
            color = (220, 20, 20) if x < 64 else (20, 40, 220)
            row.extend(color)
        rows.append(bytes(row))
    def chunk(kind, data):
        return struct.pack("!I", len(data)) + kind + data + struct.pack("!I", binascii.crc32(kind + data) & 0xffffffff)
    signature = bytes([137, 80, 78, 71, 13, 10, 26, 10])
    return signature + chunk(b"IHDR", struct.pack("!2I5B", width, height, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(b"".join(rows))) + chunk(b"IEND", b"")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def execute(account, label, content, input_items=None):
    token = account["credentials"]["access_token"]
    account_id = account["credentials"]["chatgpt_account_id"]
    request_id = str(uuid.uuid4())
    payload = {"model": account["upstream_model"], "model_selection": "explicit", "stream": True, "store": False, "reasoning_effort": "low", "input": [{"role": "developer", "type": "message", "content": [{"type": "input_text", "text": "Reply with only the requested answer. Do not call tools."}]}, {"role": "user", "type": "message", "content": content}], "metadata": {"task_id": request_id, "turn_id": request_id, "agent_iteration": "0"}}
    if input_items is not None:
        payload["input"] = input_items
    headers = {"Authorization": "Bearer " + token, "Chatgpt-Account-Id": account_id, "X-Openai-Account-Id": account_id, "X-Basispoints-Auth-Mode": "chatgpt", "Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://bps.openai.com", "User-Agent": "Mozilla/5.0", "X-Openai-Internal-Basispoints-Client-Product": "basispoints-excel-plugin", "X-Openai-Internal-Basispoints-Client-Agent-Profile": "excel"}
    req = urllib.request.Request("https://bps.openai.com/basispoints/api/responses", data=json.dumps(payload).encode(), headers=headers, method="POST")
    result = {"case": label, "account_id": account["id"], "model": account["upstream_model"], "transport": "BPS", "time_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    try:
        with urllib.request.build_opener(NoRedirect()).open(req, timeout=90) as response:
            result["http_status"] = response.status
            for raw in response:
                line = raw.decode("utf-8", errors="replace").strip()
                if not line.startswith("data:") or line[5:].strip() == "[DONE]":
                    continue
                event = json.loads(line[5:])
                kind = event.get("type")
                if kind in ("response.completed", "response.failed", "response.incomplete"):
                    final = event.get("response", {})
                    result["terminal_event"] = kind
                    result["response_status"] = final.get("status")
                    result["answer"] = "".join(part.get("text", "") for item in final.get("output", []) if item.get("type") == "message" for part in item.get("content", []) if part.get("type") == "output_text")[:500]
                    if final.get("error"):
                        result["error"] = final["error"]
                    break
                if kind == "error":
                    result["error"] = event.get("error", {"code": event.get("code"), "message": event.get("message")})
                    break
    except urllib.error.HTTPError as exc:
        result["http_status"] = exc.code
        raw = exc.read(10000).decode("utf-8", errors="replace")
        try:
            result["error"] = json.loads(raw).get("error", {})
        except json.JSONDecodeError:
            result["error_type"] = "non_json_http_error"
    except Exception as exc:
        result["error_type"] = type(exc).__name__
    safe = json.dumps(result, ensure_ascii=False).replace(token, "<REDACTED>").replace(account_id, "<REDACTED>")
    safe = re.sub(r"data:image/[^ ]+", "<REDACTED_IMAGE>", safe) if "data:image/" in safe else safe
    return json.loads(safe)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run", action="store_true", help="Send synthetic probes to the selected BPS account")
    args = parser.parse_args()
    if not args.run:
        parser.error("Pass --run to send protocol probes")
    account = credentials()
    results = []
    control = execute(account, "text_control", [{"type": "input_text", "text": "Return exactly: BPS_IMAGE_CONTROL_OK"}])
    results.append(control)
    print(json.dumps(control, ensure_ascii=False), flush=True)
    if control.get("answer", "").strip() == "BPS_IMAGE_CONTROL_OK":
        uri = "data:image/png;base64," + base64.b64encode(png_image()).decode()
        image_result = execute(account, "user_inline_png", [{"type": "input_text", "text": "Name the dominant color in the LEFT half of the image and then the RIGHT half. Reply as two English color names separated by a comma."}, {"type": "input_image", "image_url": uri, "detail": "high"}])
        image_result["expected_answer"] = "red, blue"
        results.append(image_result)
        print(json.dumps(image_result, ensure_ascii=False), flush=True)
    (ROOT / "protocol-probe-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + chr(10), encoding="utf-8")


if __name__ == "__main__":
    main()
