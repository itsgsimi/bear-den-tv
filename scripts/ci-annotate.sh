#!/usr/bin/env bash
# CI only (.github/workflows/ci.yml): turn the failures in `make test` /
# `make lint` logs into GitHub error annotations. Annotations are public on
# a public repository, while raw job logs need admin rights to download, so
# this makes a red run readable by anyone. Prints at most 10 annotations
# (GitHub shows 10 errors per step); the full log stays in the job output.
#   scripts/ci-annotate.sh LOG [LOG...]
set -euo pipefail
python3 - "$@" <<'EOF'
import re, sys

ansi = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")

def esc(s):
    return s.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")

found = []
for path in sys.argv[1:]:
    try:
        lines = [ansi.sub("", l.rstrip("\n")) for l in open(path, errors="replace")]
    except OSError:
        continue
    for i, line in enumerate(lines):
        m = re.match(r"\s*--- FAIL: (\S+)", line)  # Go test
        if m:
            detail = [l.strip() for l in lines[i + 1:i + 8] if l.startswith("    ")][:5]
            found.append(("Go test failed: " + m.group(1), "\n".join(detail) or line))
            continue
        m = re.match(r"\s*FAIL!\s*:\s*(\S+)\s*(.*)", line)  # QtTest (tst_shell)
        if m:
            loc = [l.strip() for l in lines[i + 1:i + 4] if l.strip().startswith(("Loc:", "Actual", "Expected"))]
            found.append(("Shell test failed: " + m.group(1), "\n".join([m.group(2)] + loc)))
            continue
        if re.match(r"\s*panic: ", line):  # Go panic
            found.append(("Go panic", "\n".join(lines[i:i + 6])))
            continue
        m = re.match(r"\s*(FAIL|×|✗)\s+(.+)", line)  # vitest / playwright
        if m and not re.match(r"FAIL\s+bear-den-tv/", line) and line.strip() != "FAIL":
            found.append(("Test failed", m.group(2)))
            continue
        if re.search(r"\*\*\*Failed|Test +#\d+: .*\*\*\*", line):  # ctest
            found.append(("Shell test failed", line.strip()))
            continue
        if re.match(r"make(\[\d+\])?: \*\*\* ", line):
            found.append(("make failed", "\n".join(l for l in lines[max(0, i - 6):i + 1])))

seen = set()
out = 0
for title, msg in found:
    key = (title, msg[:200])
    if key in seen:
        continue
    seen.add(key)
    print(f"::error title={esc(title)}::{esc(msg[:1500])}")
    out += 1
    if out == 10:
        break
if out == 0:
    print("::error title=Checks failed::No recognised failure lines; see the job log.")
EOF
