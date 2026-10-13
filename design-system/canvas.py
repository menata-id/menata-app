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

  python3 -I design-system/canvas.py compare design-system/tokens.json design-system/dtcg/default/menata.resolver.json
      Reads a W3C DTCG 2025.10 export (rencana 3.4, `make tokens` / `go run ./cmd/tokens`) through its resolver and
      compares every token it shares with tokens.json: colours in both themes, spacing, radius and type styles (size, line
      height, weight). Exit 1 on any difference.

Wherever a tokens.json path is taken, a DTCG `*.resolver.json` may be given instead: its `base` set and each context of
its `mode` modifier become the same families (colour per theme, spacing, radius), so a mockup for one Workspace's own
theme is generated from that Workspace's export. It carries only the tokens the runtime's readers answer for, not the
artifact's unmapped ones (brand, focus-ring, the shadows).

Standard library only; run with `python3 -I` (see the repo's CLAUDE.md on running Python over downloaded files).
"""
import json
import re
import sys
from collections import Counter
from pathlib import Path

FAMILIES = ["color", "spacing", "radius", "shadow"]
ROOT_RE = re.compile(r":root\{.*?\n\}", re.S)
VERSION_RE = re.compile(r"\((?:tokens\.json|DTCG export [^,]*), version [^,]*, [^)]*\)")


def first_theme(v):
    """A token's value in the first theme (light). Since dark mode (2026-10-13) a colour may be {"light": ..., "dark": ...};
    the canvases are light, so they take the first theme, as the Design System page does."""
    if isinstance(v, dict):
        return v.get("light", next(iter(v.values())))
    return v


def load_tokens(path):
    """tokens.json, or a DTCG 2025.10 resolver document read into tokens.json's shape."""
    if str(path).endswith(".resolver.json"):
        return dtcg_tokens(Path(path))
    return json.loads(Path(path).read_text())


def dtcg_tokens(resolver_path):
    """Read a DTCG export through its resolver: every source of every set and of every modifier context, in
    resolutionOrder. Colours come out as {theme: value}, one theme per `mode` context; dimensions as `<n>px`."""
    resolver = json.loads(resolver_path.read_text())
    if resolver.get("version") != "2025.10":
        sys.exit("%s: not a DTCG 2025.10 resolver" % resolver_path)
    families = {fam: {"tokens": []} for fam in FAMILIES}
    families["_type"] = {}
    colours = {}

    def sources(ref):
        target = ref["$ref"]
        if not target.startswith("#/"):
            return [(None, target)]
        kind, name = target[2:].split("/", 1)
        node = resolver[kind][name]
        if kind == "sets":
            return [(None, s["$ref"]) for s in node["sources"]]
        return [(ctx, s["$ref"]) for ctx, refs in node["contexts"].items() for s in refs]

    for entry in resolver["resolutionOrder"]:
        for ctx, file in sources(entry):
            doc = json.loads((resolver_path.parent / file).read_text())
            for group, tokens in doc.items():
                if group.startswith("$"):
                    continue
                for name, tok in tokens.items():
                    if name.startswith("$"):
                        continue
                    kind = tok.get("$type", tokens.get("$type"))
                    if kind == "color":
                        colours.setdefault(name, {})[ctx or "light"] = dtcg_colour(tok["$value"])
                    elif kind == "typography":
                        v = tok["$value"]
                        size = v["fontSize"]["value"]
                        families["_type"][name] = {"fontSize": "%gpx" % size, "lineHeight": round(v["lineHeight"] * size, 2),
                                                   "fontWeight": v["fontWeight"]}
                    elif kind == "dimension" and group in families:
                        v = tok["$value"]
                        families[group]["tokens"].append({"name": name, "value": "%g%s" % (v["value"], v["unit"])})
    families["color"]["tokens"] = [{"name": n, "value": v} for n, v in sorted(colours.items())]
    families["_source"] = resolver_path.name
    return families


def dtcg_colour(v):
    if v["colorSpace"] == "oklch":
        l, c, h = v["components"]
        return "oklch(%s%% %s %s)" % (fmt_num(l * 100), fmt_num(c), fmt_num(h))
    return v["hex"]


def fmt_num(f):
    return ("%.4f" % f).rstrip("0").rstrip(".")


def resolve(tokens, name, theme):
    """A colour token's value in one theme, following `{alias}` chains the way the artifact does."""
    by_name = {t["name"]: t["value"] for t in tokens["color"]["tokens"]}
    for _ in range(16):
        v = by_name[name]
        v = v.get(theme, v.get("light")) if isinstance(v, dict) else v
        m = re.fullmatch(r"\{([\w-]+)\}", str(v))
        if not m:
            return v
        name = m.group(1)
    sys.exit("alias chain from %s is too deep" % name)


def compare(ds, export):
    bad, n = [], 0
    ds_names = {t["name"] for t in ds["color"]["tokens"]}
    for t in export["color"]["tokens"]:
        if t["name"] not in ds_names:
            bad.append("%s: in the export, not in tokens.json" % t["name"])
            continue
        for theme in t["value"]:
            got, want = norm(t["value"][theme]), norm(resolve(ds, t["name"], theme))
            n += 1
            if got != want:
                bad.append("%s (%s): export %s, tokens.json %s" % (t["name"], theme, got, want))
    for fam in ("spacing", "radius"):
        ds_vals = {t["name"]: t["value"] for t in ds[fam]["tokens"]}
        for t in export[fam]["tokens"]:
            n += 1
            if ds_vals.get(t["name"]) != t["value"]:
                bad.append("%s: export %s, tokens.json %s" % (t["name"], t["value"], ds_vals.get(t["name"])))
    styles = {st["name"]: st for g in ds["type"]["groups"] for st in g["styles"]}
    for name, got in sorted(export.get("_type", {}).items()):
        st = styles.get(name)
        n += 1
        if st is None:
            bad.append("typography.%s: in the export, not in tokens.json" % name)
            continue
        size = float(st["fontSize"].rstrip("px"))
        lh = st["lineHeight"]
        lh = float(lh.rstrip("px")) if isinstance(lh, str) else round(lh * size, 2)
        want = {"fontSize": st["fontSize"], "lineHeight": lh, "fontWeight": st["fontWeight"]}
        if got != want:
            bad.append("typography.%s: export %s, tokens.json %s" % (name, got, want))
    print("compared %d token values (colours per theme, spacing, radius, type styles)" % n)
    for b in bad:
        print("FAIL " + b)
    return 1 if bad or n == 0 else 0


def css_value(v):
    v = first_theme(v)
    m = re.fullmatch(r"\{([\w-]+)\}", str(v))
    return "var(--%s)" % m.group(1) if m else v


def block(tokens, version, date):
    lines = [
        '  /* Menata App design system: generated from the Design System artifact "Menata App"',
        "     (tokens.json, version %s, %s). Edit the Design System, not this block. */" % (version, date),
    ]
    if "_source" in tokens:
        lines = [
            "  /* Menata App design system: generated from the runtime's own Theme, exported as DTCG tokens",
            "     (DTCG export %s, version %s, %s). Edit the Theme and re-export, not this block. */" % (tokens["_source"], version, date),
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
            strip = lambda x: ROOT_RE.sub("", x, count=1)
            if strip(out) != strip(s):
                sys.exit("%s: something outside the :root block would change -- refusing" % f.name)
        elif "<style>\n" in s:
            # A new artboard: the block and one blank line go in after `<style>`, and nothing else moves. Checked by
            # taking exactly that back out (comparing with the block stripped refused every insertion until 2026-10-13).
            out = s.replace("<style>\n", "<style>\n" + new_block + "\n\n", 1)
            if out.replace(new_block + "\n\n", "", 1) != s:
                sys.exit("%s: inserting the :root block would change something else -- refusing" % f.name)
        else:
            sys.exit("%s: no :root block and no <style> to insert one into" % f.name)
        if out != s:
            f.write_text(out)
            changed += 1
    print("generated %d of %d artboards (version %s)" % (changed, len(boards(folder)), version))


def check(tokens, folder):
    values = set()
    for fam in ["color"]:
        for t in tokens[fam]["tokens"]:
            v = str(first_theme(t["value"]))
            if not v.startswith("{"):
                values.add(norm(v))
    bad, warn, refs, on, off = [], [], Counter(), 0, Counter()
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
        # A reference no block token and no declaration in the file defines draws nothing. Generating from a DTCG export
        # carries fewer tokens than tokens.json (no brand, focus-ring or shadows), so this is where that shows.
        # A warning, not a failure: it was added 2026-10-13 without being run over the live canvas, so what it reports
        # there is unmeasured. Promote it once a run over the canvas shows zero.
        defined = set(re.findall(r"--([\w-]+)\s*:", s))
        for name in sorted(set(re.findall(r"var\(--([\w-]+)", body)) - defined):
            warn.append("%s: var(--%s) is defined by no token and nothing in the file" % (f.name, name))
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
    print("undefined var() references: %d" % len(warn))
    for w in warn:
        print("WARN " + w)
    for b in bad:
        print("FAIL " + b)
    return 1 if bad else 0


def norm(v):
    v = " ".join(v.lower().split())
    v = re.sub(r"([( ,/])0\.", r"\1.", v)
    return {"#ffffff": "#fff"}.get(v, v)


def main(argv):
    if len(argv) < 4 or argv[1] not in ("generate", "check", "compare"):
        sys.exit(__doc__)
    tokens = load_tokens(argv[2])
    if argv[1] == "compare":
        return compare(tokens, load_tokens(argv[3]))
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
