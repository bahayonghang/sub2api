"""Verify images in the exact tool-result content position."""
import argparse
import base64
import json
from pathlib import Path

import probe_bps_images as probe


def items_for(output, prompt):
    call_id = "call_bps_image_probe"
    return [{"role": "developer", "content": [{"type": "input_text", "text": "Use the provided tool result to answer. Do not call any more tools."}]}, {"role": "user", "content": [{"type": "input_text", "text": prompt}]}, {"type": "function_call", "id": "fc_bps_image_probe", "call_id": call_id, "name": "run_officejs", "arguments": json.dumps({"code": "synthetic_image", "summary": "Read synthetic image", "extended_summary": "Return a synthetic image for verification", "destructive": False, "references": []}), "status": "completed"}, {"type": "function_call_output", "id": "fc_bps_image_result", "call_id": call_id, "output": output}]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run", action="store_true")
    if not parser.parse_args().run:
        parser.error("Pass --run to send synthetic tool-result probes")
    account = probe.credentials()
    results = []
    output = [{"type": "input_text", "text": "BPS_TOOL_CONTROL_OK"}]
    control = probe.execute(account, "tool_text_array_control", [], input_items=items_for(output, "Reply with exactly the provided tool result text."))
    results.append(control)
    print(json.dumps(control, ensure_ascii=False), flush=True)
    if control.get("answer", "").strip() == "BPS_TOOL_CONTROL_OK":
        uri = "data:image/png;base64," + base64.b64encode(probe.png_image()).decode()
        prompt = "Name the dominant color in the LEFT half of the image and then the RIGHT half. Reply as two English color names separated by a comma."
        for detail in ["auto", "high", "original"]:
            parts = [{"type": "input_text", "text": "Synthetic test image attached."}, {"type": "input_image", "image_url": uri, "detail": detail}]
            items = items_for(parts, prompt)
            result = probe.execute(account, "tool_inline_png_" + detail, [], input_items=items)
            result["expected_answer"] = "red, blue"
            results.append(result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
            if detail == "auto" and result.get("response_status") == "completed":
                items.extend([{"role": "assistant", "content": [{"type": "output_text", "text": result.get("answer", "")}]}, {"role": "user", "content": [{"type": "input_text", "text": "Looking at the same earlier image, name only the color on its right half."}]}])
                history = probe.execute(account, "tool_inline_history_png", [], input_items=items)
                history["expected_answer"] = "blue"
                results.append(history)
                print(json.dumps(history, ensure_ascii=False), flush=True)
    Path(__file__).with_name("tool-image-probe-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + chr(10), encoding="utf-8")


if __name__ == "__main__":
    main()
