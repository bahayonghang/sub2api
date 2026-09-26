"""Run the current Go adapter with synthetic pixels and the selected BPS account."""
import argparse
import json
import os
from pathlib import Path
import subprocess
from probe_bps_images import credentials

ROOT = Path(__file__).resolve().parent
REPO = next(parent for parent in ROOT.parents if (parent / "backend/go.mod").is_file())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run", action="store_true")
    args = parser.parse_args()
    if not args.run:
        parser.error("Pass --run to send synthetic image requests")
    probe = REPO / "backend/internal/service/openai_excel_bps_live_probe_test.go"
    if probe.exists():
        raise RuntimeError("temporary_probe_file_already_exists")
    account = credentials()
    env = os.environ.copy()
    env["SUB2API_BPS_LIVE_ACCOUNT"] = json.dumps(account)
    source = (ROOT / "adapter-probe.go.txt").read_text(encoding="utf-8")
    try:
        probe.write_text(source, encoding="utf-8")
        completed = subprocess.run(["go", "test", "./internal/service", "-run", "^TestExcelBPSLiveLocalImageProbe$", "-count=1", "-v", "-timeout", "10m"], cwd=REPO / "backend", env=env, capture_output=True, text=True, encoding="utf-8", errors="replace")
        results = []
        for line in completed.stdout.splitlines():
            marker = "BPS_ADAPTER_RESULT "
            if marker in line:
                results.append(json.loads(line.split(marker,1)[1]))
            elif line.startswith(("---", "PASS", "FAIL", "ok", "    ---")):
                print(line, flush=True)
        print(json.dumps({"exit_code":completed.returncode,"results":results},ensure_ascii=False),flush=True)
        (ROOT / "adapter-probe-results.json").write_text(json.dumps({"exit_code":completed.returncode,"results":results},ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
        if completed.returncode:
            safe = completed.stdout + completed.stderr
            for value in account["credentials"].values():
                if isinstance(value,str) and value:
                    safe = safe.replace(value,"[redacted]")
            safe = safe.replace(env["SUB2API_BPS_LIVE_ACCOUNT"],"[redacted]")
            print(safe[-8000:],flush=True)
        raise SystemExit(completed.returncode)
    finally:
        probe.unlink(missing_ok=True)


if __name__ == "__main__":
    main()
