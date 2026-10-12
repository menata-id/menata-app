#!/usr/bin/env python3
"""GS6 of menata-app-document's guides/menjaga-design-system.md: the mockup canvas is generated from the Design
System artifact, and this script both generates it and checks it.

The canvas is not in this repository, so this is a tool rather than a CI gate. An agent first downloads the canvas's
`project/*.dc.html` files (the Artifact tool's `read` with `paths`) into a folder, then runs one of the modes below,
then publishes what `generate` rewrote. Rules R3 and R8 of the guide say when to run it: on every token change.

  python3 -I design-system/canvas.py generate design-system/tokens.json <canvas>/project --version <artifact-version>
      Rewrites the `:root{...}` block of every artboard from tokens.json (inserting it where missing) and verifies that
      nothing outside the block changed.

  python3 -I design-system/canvas.py check design-system/tokens.json <canvas>/project
      Fails (exit 1) if any artboard's block differs from a fresh generation, ignoring the version comment. Also
      reports the adoption metrics: `var(--…)` references per artboard, and colour literals outside the block that
      equal a token value versus those that do not (off-system colours).

Standard library only; run with `python3 -I` (see the repo's CLAUDE.md on running Python over downloaded files).
"""
import json
import re
import sys
from collections import Counter
from pathlib import Path

FAMILIES = ["color", "spacing", "radius", "shadow"]
ROOT_RE = re.compile(r":root\{.*?\n\}", re.S)
VERSION_RE = re.compile(r"\(tokens\.json, version [^,]*, [^)]*\)")


def css_value(v):
    m = re.fullmatch(r"\{([\w-]+)\}", str(v))
    return "var(--%s)" % m.group(1) if m else v


def block(tokens, version, date):
    lines = [
        '  /* Menata App design system: generated from the Design System artifact "Menata App"',
        "     (tokens.json, version %s, %s). Edit the Design System, not this block. */" % (version, date),
    ]
    for fam in FAMILIES:
        items = ["--%s:%s;" % (t["name"], css_value(t["value"])) for t in tokens[fam]["tokens"]]
        lines.append("  /* %s */" % fam)
        row = ""
        for it in items:
            if len(row) + len(it) > 110:
                lines.append("  " + row.rstrip())
                row = ""
            row += it + " "
        if row:
            lines.append("  " + row.rstrip())
    return ":root{\n" + "\n".join(lines) + "\n}"


def boards(folder):
    files = sorted(Path(folder).glob("*.dc.html"))
    if not files:
        sys.exit("no *.dc.html in %s" % folder)
    return files


def generate(tokens, folder, version, date):
    new_block = block(tokens, version, date)
    changed = 0
    for f in boards(folder):
        s = f.read_text()
        m = ROOT_RE.search(s)
        if m:
            out = s[: m.start()] + new_block + s[m.end():]
        elif "<style>\n" in s:
            out = s.replace("<style>\n", "<style>\n" + new_block + "\n\n", 1)
        else:
            sys.exit("%s: no :root block and no <style> to insert one into" % f.name)
        strip = lambda x: ROOT_RE.sub("", x, count=1).replace("\n\n", "\n")
        if strip(out) != strip(s):
            sys.exit("%s: something outside the :root block would change -- refusing" % f.name)
        if out != s:
            f.write_text(out)
            changed += 1
    print("generated %d of %d artboards from tokens.json (version %s)" % (changed, len(boards(folder)), version))


def check(tokens, folder):
    values = set()
    for fam in ["color"]:
        for t in tokens[fam]["tokens"]:
            v = str(t["value"])
            if not v.startswith("{"):
                values.add(norm(v))
    bad, refs, on, off = [], Counter(), 0, Counter()
    for f in boards(folder):
        s = f.read_text()
        m = ROOT_RE.search(s)
        if not m:
            bad.append("%s: no :root block" % f.name)
            continue
        got = VERSION_RE.sub("", m.group(0))
        want = VERSION_RE.sub("", block(tokens, "x", "x"))
        if got != want:
            bad.append("%s: :root block differs from tokens.json" % f.name)
        body = s[m.end():]
        refs[f.name] = len(re.findall(r"var\(--", body))
        for lit in re.findall(r"oklch\([^)]*\)|#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b", body):
            if norm(lit) in values:
                on += 1
            else:
                off[lit.lower()] += 1
    total_refs = sum(refs.values())
    print("artboards: %d; blocks out of date: %d" % (len(refs) + sum(1 for b in bad if "no :root" in b), len(bad)))
    print("var(--) references: %d, in %d artboard(s): %s" % (total_refs, sum(1 for v in refs.values() if v),
          ", ".join("%s=%d" % (k, v) for k, v in refs.most_common() if v)))
    print("colour literals outside the block: %d equal a token value, %d do not" % (on, sum(off.values())))
    if off:
        print("top off-token literals: " + ", ".join("%s x%d" % kv for kv in off.most_common(10)))
    for b in bad:
        print("FAIL " + b)
    return 1 if bad else 0


def norm(v):
    v = " ".join(v.lower().split())
    v = re.sub(r"([( ,/])0\.", r"\1.", v)
    return {"#ffffff": "#fff"}.get(v, v)


def main(argv):
    if len(argv) < 4 or argv[1] not in ("generate", "check"):
        sys.exit(__doc__)
    tokens = json.loads(Path(argv[2]).read_text())
    if argv[1] == "generate":
        if "--version" not in argv:
            sys.exit("generate needs --version <artifact-version>")
        version = argv[argv.index("--version") + 1]
        date = argv[argv.index("--date") + 1] if "--date" in argv else __import__("datetime").date.today().isoformat()
        generate(tokens, argv[3], version, date)
        return 0
    return check(tokens, argv[3])


if __name__ == "__main__":
    sys.exit(main(sys.argv))
