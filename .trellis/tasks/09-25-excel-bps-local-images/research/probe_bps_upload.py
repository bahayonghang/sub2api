"""Verify the official BPS attachment contract with one disposable image."""
import argparse
import json
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

import probe_bps_images as probe


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run", action="store_true")
    if not parser.parse_args().run:
        parser.error("Pass --run to upload and delete a synthetic test image")
    account = probe.credentials()
    token = account["credentials"]["access_token"]
    account_id = account["credentials"]["chatgpt_account_id"]
    headers = {"Authorization": "Bearer " + token, "Chatgpt-Account-Id": account_id, "X-Openai-Account-Id": account_id, "X-Basispoints-Auth-Mode": "chatgpt", "Origin": "https://bps.openai.com", "User-Agent": "Mozilla/5.0", "X-Openai-Internal-Basispoints-Client-Product": "basispoints-excel-plugin", "X-Openai-Internal-Basispoints-Client-Agent-Profile": "excel"}
    boundary = "BPSProbe" + uuid.uuid4().hex
    crlf = bytes([13, 10])
    body = ("--" + boundary).encode() + crlf + b'Content-Disposition: form-data; name="file"; filename="synthetic-colors.png"' + crlf + b"Content-Type: image/png" + crlf + crlf + probe.png_image() + crlf + ("--" + boundary + "--").encode() + crlf
    opener = urllib.request.build_opener(probe.NoRedirect())
    records = []
    file_id = None
    try:
        req = urllib.request.Request("https://bps.openai.com/basispoints/api/attachments", data=body, headers={**headers, "Content-Type": "multipart/form-data; boundary=" + boundary}, method="POST")
        with opener.open(req, timeout=90) as response:
            data = json.load(response)
            file_id = data.get("openai_file_id")
            records.append({"case": "attachment_upload", "http_status": response.status, "file_id_present": bool(file_id), "response_fields": sorted(data), "content_type": data.get("content_type"), "size": data.get("size")})
        if not file_id:
            raise RuntimeError("attachment_response_missing_file_id")
        question = {"type": "input_text", "text": "Name the dominant color in the LEFT half of the image and then the RIGHT half. Reply as two English color names separated by a comma."}
        for detail in ["auto", "high", "original"]:
            result = probe.execute(account, "uploaded_user_png_" + detail, [question, {"type": "input_image", "file_id": file_id, "detail": detail}])
            result["expected_answer"] = "red, blue"
            records.append(result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
        call_id = "call_bps_image_probe"
        items = [{"role": "developer", "content": [{"type": "input_text", "text": "Use the provided tool image to answer. Do not call any more tools."}]}, {"role": "user", "content": [question]}, {"type": "function_call", "id": "fc_bps_image_probe", "call_id": call_id, "name": "run_officejs", "arguments": json.dumps({"code": "synthetic_image", "summary": "Read synthetic image", "extended_summary": "Return a synthetic image for verification", "destructive": False, "references": []}), "status": "completed"}, {"type": "function_call_output", "call_id": call_id, "output": [{"type": "input_text", "text": "Synthetic test image attached."}, {"type": "input_image", "file_id": file_id, "detail": "high"}]}]
        tool_result = probe.execute(account, "uploaded_tool_png", [], input_items=items)
        tool_result["expected_answer"] = "red, blue"
        records.append(tool_result)
        print(json.dumps(tool_result, ensure_ascii=False), flush=True)
        if tool_result.get("response_status") == "completed":
            items.extend([{"role": "assistant", "content": [{"type": "output_text", "text": tool_result.get("answer", "")}]}, {"role": "user", "content": [{"type": "input_text", "text": "Looking at the same earlier image, name only the color on its right half."}]}])
            history = probe.execute(account, "uploaded_history_png", [], input_items=items)
            history["expected_answer"] = "blue"
            records.append(history)
            print(json.dumps(history, ensure_ascii=False), flush=True)
    except urllib.error.HTTPError as exc:
        records.append({"case": "attachment_transport", "http_status": exc.code})
    finally:
        if file_id:
            url = "https://bps.openai.com/basispoints/api/attachments/delete/" + urllib.parse.quote(file_id, safe="")
            try:
                with opener.open(urllib.request.Request(url, headers=headers, method="DELETE"), timeout=30) as response:
                    records.append({"case": "attachment_cleanup", "http_status": response.status})
            except urllib.error.HTTPError as exc:
                records.append({"case": "attachment_cleanup", "http_status": exc.code})
        output = Path(__file__).with_name("upload-probe-results.json")
        output.write_text(json.dumps(records, ensure_ascii=False, indent=2) + chr(10), encoding="utf-8")
        print(json.dumps({"upload_and_cleanup": [entry for entry in records if entry["case"].startswith("attachment_")]}, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
