#!/usr/bin/env bash
# Verify docs site link integrity before GitHub Pages deploy.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== edgeCore docs site verification =="
echo "Root: $ROOT"
echo

echo "1. Internal link audit..."
python3 scripts/audit_links.py
echo

echo "2. Landing page link check..."
python3 - <<'PY'
import re
import urllib.parse
from pathlib import Path

docs = Path(".")
landing = (docs / "_includes/landing-home.html").read_text(encoding="utf-8")
links = re.findall(r'href="([^"]+)"', landing)
missing = []
for link in links:
    if link.startswith("http"):
        continue
    path = urllib.parse.unquote(link.split("#")[0])
    html = docs / path
    md = html.with_suffix(".md")
    if not html.exists() and not md.exists():
        missing.append(link)

print(f"Landing links: {len(links)}")
if missing:
    print(f"Missing targets: {len(missing)}")
    for m in missing:
        print(f"  - {m}")
    raise SystemExit(1)
print("All landing links resolve to source files.")
PY

echo
echo "3. Asset files..."
for f in assets/style.css assets/script.js; do
  test -f "$f" || { echo "Missing $f"; exit 1; }
done
echo "CSS/JS present."

echo
echo "4. Jekyll build (optional)..."
if command -v bundle >/dev/null 2>&1 && bundle exec jekyll --version >/dev/null 2>&1; then
  bundle exec jekyll build --trace
  echo "Jekyll build OK -> _site/"
else
  echo "Skipped (run: cd docs && bundle install && bundle exec jekyll build)"
fi

echo
echo "Verification complete."
