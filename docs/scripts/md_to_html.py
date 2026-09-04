#!/usr/bin/env python3
"""Minimal Markdown→HTML for Jekyll-style docs when bundle exec jekyll is unavailable."""
from __future__ import annotations
import html
import re
import sys
from pathlib import Path


def inline(text: str) -> str:
    text = html.escape(text)
    text = re.sub(r"`([^`]+)`", r"<code>\1</code>", text)
    text = re.sub(r"\*\*([^*]+)\*\*", r"<strong>\1</strong>", text)
    text = re.sub(
        r"\[([^\]]+)\]\(([^)]+)\)",
        lambda m: f'<a href="{html.escape(m.group(2), quote=True)}">{html.escape(m.group(1))}</a>',
        text,
    )
    return text


def is_table_row(line: str) -> bool:
    s = line.strip()
    return s.startswith("|") and s.endswith("|")


def is_sep_row(line: str) -> bool:
    return bool(re.match(r"^\|\s*:?-+:?\s*(\|\s*:?-+:?\s*)+\|\s*$", line.strip()))


def parse_table(lines: list[str]) -> str:
    rows = [ [c.strip() for c in ln.strip().strip("|").split("|")] for ln in lines ]
    align = []
    if len(rows) > 1 and is_sep_row("|".join(["|"] + rows[1] + ["|"])):
        for cell in rows[1]:
            c = cell.strip()
            if c.startswith(":") and c.endswith(":"):
                align.append("center")
            elif c.endswith(":"):
                align.append("right")
            else:
                align.append("left")
        body = rows[2:]
        header = rows[0]
    else:
        header, body = rows[0], rows[1:]
        align = ["left"] * len(header)

    out = ["<table>", "<thead><tr>"]
    for i, cell in enumerate(header):
        out.append(f'<th style="text-align:{align[i] if i < len(align) else "left"}">{inline(cell)}</th>')
    out.append("</tr></thead><tbody>")
    for row in body:
        out.append("<tr>")
        for i, cell in enumerate(row):
            out.append(f'<td style="text-align:{align[i] if i < len(align) else "left"}">{inline(cell)}</td>')
        out.append("</tr>")
    out.append("</tbody></table>")
    return "\n".join(out)


def convert(md: str) -> str:
    if md.startswith("---"):
        md = md.split("---", 2)[2].lstrip("\n")

    lines = md.splitlines()
    out: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        if stripped.startswith("```"):
            lang = stripped[3:].strip()
            i += 1
            block = []
            while i < len(lines) and not lines[i].strip().startswith("```"):
                block.append(html.escape(lines[i]))
                i += 1
            cls = f' class="language-{lang}"' if lang else ""
            out.append(f"<pre><code{cls}>{chr(10).join(block)}</code></pre>")
            i += 1
            continue

        if is_table_row(line):
            tbl = [line]
            i += 1
            while i < len(lines) and is_table_row(lines[i]):
                tbl.append(lines[i])
                i += 1
            out.append(parse_table(tbl))
            continue

        if stripped == "---":
            out.append("<hr />")
        elif stripped.startswith("#### "):
            out.append(f"<h4>{inline(stripped[5:])}</h4>")
        elif stripped.startswith("### "):
            out.append(f"<h3>{inline(stripped[4:])}</h3>")
        elif stripped.startswith("## "):
            out.append(f"<h2>{inline(stripped[3:])}</h2>")
        elif stripped.startswith("# "):
            out.append(f"<h1>{inline(stripped[2:])}</h1>")
        elif stripped.startswith("> "):
            out.append(f"<blockquote><p>{inline(stripped[2:])}</p></blockquote>")
        elif stripped.startswith("- "):
            out.append("<ul>")
            while i < len(lines) and lines[i].strip().startswith("- "):
                out.append(f"<li>{inline(lines[i].strip()[2:])}</li>")
                i += 1
            out.append("</ul>")
            continue
        elif stripped == "":
            pass
        else:
            out.append(f"<p>{inline(stripped)}</p>")

        i += 1

    return "\n".join(out)


def wrap(title: str, description: str, content: str, depth: int) -> str:
    root = "../" * depth
    nav = [
        ("返回项目", "https://github.com/anviod/edgeCore"),
        ("首页", f"{root}index.html"),
        ("产品指南", f"{root}guide/index.html"),
        ("设备驱动", f"{root}drivers/index.html"),
        ("测试验证", f"{root}testing/index.html"),
        ("边缘计算", f"{root}edge/index.html"),
        ("开发计划", f"{root}development_plan/index.html"),
    ]
    nav_html = "\n".join(f'              <a href="{url}">{label}</a>' for label, url in nav)
    desc = html.escape(description)
    return f"""<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{html.escape(title)} | edgeCore 知识库</title>
    <meta name="description" content="{desc}">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Noto+Sans+SC:wght@400;500;700;800&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="{root}assets/style.css">
  </head>
  <body class="page-docs">
    <header class="site-header site-header--docs">
      <div class="shell shell--wide">
        <div class="site-header__topbar">
          <a class="site-brand" href="{root}index.html">edgeCore 知识库</a>
          <nav class="site-nav" aria-label="主导航">
{nav_html}
          </nav>
        </div>
        <div class="site-header__intro"></div>
      </div>
    </header>
    <main class="page-main">
      <div class="shell shell--wide">
        <article class="markdown-body">
          {content}
        </article>
      </div>
    </main>
    <script src="{root}assets/script.js"></script>
  </body>
</html>
"""


def main() -> None:
    if len(sys.argv) != 3:
        print("usage: md_to_html.py input.md output.html", file=sys.stderr)
        sys.exit(1)

    src = Path(sys.argv[1])
    dst = Path(sys.argv[2])
    raw = src.read_text(encoding="utf-8")

    title = "Document"
    description = ""
    fm = re.match(r"^---\n(.*?)\n---\n", raw, re.S)
    if fm:
        for line in fm.group(1).splitlines():
            if line.startswith("title:"):
                title = line.split(":", 1)[1].strip()
            elif line.startswith("description:"):
                description = line.split(":", 1)[1].strip()

    parts = dst.parts
    if "docs" in parts:
        idx = parts.index("docs")
        depth = len(parts) - idx - 2
    else:
        depth = 1

    body = convert(raw)
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text(wrap(title, description, body, depth), encoding="utf-8")
    print(f"wrote {dst}")


if __name__ == "__main__":
    main()
