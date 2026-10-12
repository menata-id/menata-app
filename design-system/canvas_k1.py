"""Owner decision K1 of the Phase 1 decision sheet (2026-10-12): the mockup canvas's hex colours are the Tailwind v3
palette, the Design System's tokens are the v4 palette in oklch. Replace each v3 hex with the v4 value of the same
palette step (slate-200 stays slate-200), in the artboards' bodies only. Mockup only; the code already draws v4.

  python3 -I design-system/canvas_k1.py <canvas>/project <out-folder> design-system/k1-v3-to-v4.json

The mapping file is the record: one entry per hex the canvas used, with its palette name, the v4 value and the
perceptual distance (OKLab ΔE, x100) between the two. It was built from Tailwind 3.4.19's `colors.js` and 4.1.18's
`theme.css` (whose palette equals the 4.3.3 the code compiles: every colour in `app.css` was compared). White and
black are the same in both and are not touched. A hex outside the v3 palette is left alone and counted.

Every file is verified: mapping each replaced value back to its hex must give the original file.
"""
import collections
import json
import os
import re
import sys

SRC, DST, MAPPING = sys.argv[1], sys.argv[2], sys.argv[3]
mapping = json.load(open(MAPPING))["colours"]
to_v4 = {m["hex"]: m["v4"] for m in mapping}
back_to_hex = {}
for m in mapping:
    back_to_hex.setdefault(m["v4"], m["hex"])
    if back_to_hex[m["v4"]] != m["hex"]:
        sys.exit("two hex map to one v4 value: %s, %s" % (back_to_hex[m["v4"]], m["hex"]))

hex_re = re.compile(r"#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b")
done = collections.Counter()
left = collections.Counter()

os.makedirs(DST, exist_ok=True)
for fn in sorted(os.listdir(SRC)):
    if not fn.endswith(".dc.html"):
        continue
    s = open(os.path.join(SRC, fn)).read()
    root = re.search(r":root\{.*?\n\}", s, re.S)
    head, body = s[: root.end()], s[root.end():]

    def on_hex(m):
        h = m.group(0).lower()
        if h in to_v4:
            done[fn] += 1
            return to_v4[h]
        if h not in ("#fff", "#ffffff", "#000", "#000000"):
            left[h] += 1
        return m.group(0)

    # The canvas already holds oklch literals (the 0.6 pass's and the artboards' own). They are set aside first so the
    # verification below maps back only what this pass wrote.
    kept = []

    def keep(m):
        kept.append(m.group(0))
        return "\x00%d\x00" % (len(kept) - 1)

    shielded = re.sub(r"oklch\([^)]*\)", keep, body)
    if "\x00" in body:
        sys.exit("unexpected NUL in " + fn)
    replaced = hex_re.sub(on_hex, shielded)
    # Verification: every v4 value this pass wrote maps back to its hex, giving the original body (case-folded,
    # since the original's hex case is not recorded).
    restored = re.sub(r"oklch\([^)]*\)", lambda m: back_to_hex[m.group(0)], replaced)
    if restored.lower() != shielded.lower():
        sys.exit("verification failed: " + fn)
    new_body = re.sub(r"\x00(\d+)\x00", lambda m: kept[int(m.group(1))], replaced)
    open(os.path.join(DST, fn), "w").write(head + new_body)

print("replaced", sum(done.values()), "in", len(done), "files")
print("left (not in the v3 palette)", dict(left))
