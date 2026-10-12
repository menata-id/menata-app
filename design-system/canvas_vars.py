"""Rencana 0.6 of the design-system plan: replace colour literals in the mockup canvas's artboards with var(--token)
references, purely (no colour changes).

  python3 -I design-system/canvas_vars.py <canvas>/project <out-folder> design-system/tokens.json

**Pure only.** Hex literals are deliberately not touched: the canvas's hex is the Tailwind v3 palette, the tokens are
the v4 palette in oklch, and the two differ (by up to 0.03 in OKLab for blues). Replacing them is owner decision K1 of
the Phase 1 decision sheet, not a substitution. A hex that is textually equal to a token can still mean something else
(`#2563eb` equals `brand-icon` but is v3 blue-600, a link colour), which is why text equality is not used for hex.

Only oklch literals whose value equals a Design System colour token are touched. The token name is chosen by the CSS
property's family (color -> ink, background -> fill, border/outline -> edge). A primitive token is preferred; an
alias of the same value (e.g. primary = ink-strong as a fill) is used only when no primitive is in the family. A value
with no unique candidate is left alone and counted. Every file is verified by resolving each var(--x) back to its
value and comparing with the original, normalised.
"""
import collections
import json
import os
import re
import sys

SRC, DST, TOKENS = sys.argv[1], sys.argv[2], sys.argv[3]
tok = {x["name"]: str(x["value"]) for x in json.load(open(TOKENS))["color"]["tokens"]}


def norm(v):
    v = " ".join(v.lower().split())
    return re.sub(r"([( ,/])0\.", r"\1.", v)


def resolve(v):
    m = re.fullmatch(r"\{([\w-]+)\}", v)
    return resolve(tok[m.group(1)]) if m else v


primitives = collections.defaultdict(list)
aliases = collections.defaultdict(list)
for name, value in tok.items():
    (aliases if value.startswith("{") else primitives)[norm(resolve(value))].append(name)


def family(name):
    if name.startswith("ink-") or name.endswith("-ink") or name in ("accent", "accent-hover"):
        return "ink"
    if name.startswith("border-") or name.endswith("-border") or name in ("focus-ring", "accent-ring"):
        return "edge"
    return "fill"


def property_family(prop):
    if prop == "color":
        return "ink"
    if prop.startswith(("border", "outline")):
        return "edge"
    if prop.startswith("background"):
        return "fill"
    return None


stats = collections.Counter()
skipped = collections.Counter()
decl = re.compile(r"([\w-]+)(\s*:\s*)([^;\"{}]*)")
lit_re = re.compile(r"oklch\([^)]*\)")


def back(text):
    return re.sub(r"var\(--([\w-]+)\)", lambda m: resolve(tok[m.group(1)]) if m.group(1) in tok else m.group(0), text)


os.makedirs(DST, exist_ok=True)
for fn in sorted(os.listdir(SRC)):
    if not fn.endswith(".dc.html"):
        continue
    s = open(os.path.join(SRC, fn)).read()
    root = re.search(r":root\{.*?\n\}", s, re.S)
    head, body = s[: root.end()], s[root.end():]

    def on_decl(dm):
        prop, sep, val = dm.group(1), dm.group(2), dm.group(3)
        pf = property_family(prop)

        def on_lit(lm):
            lit = lm.group(0)
            key = norm(lit)
            if key not in primitives:
                return lit
            cand = [n for n in primitives[key] if pf and family(n) == pf]
            if not cand and pf:
                cand = [n for n in aliases.get(key, []) if family(n) == pf]
            if len(cand) != 1:
                skipped[(prop, tuple(primitives[key]))] += 1
                return lit
            stats[fn] += 1
            return "var(--%s)" % cand[0]

        return prop + sep + lit_re.sub(on_lit, val)

    new_body = decl.sub(on_decl, body)
    if norm(back(new_body)) != norm(back(body)):
        sys.exit("verification failed: " + fn)
    open(os.path.join(DST, fn), "w").write(head + new_body)

print("substituted", sum(stats.values()), dict(stats))
print("skipped", sum(skipped.values()), skipped.most_common(12))
