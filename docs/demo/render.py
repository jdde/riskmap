"""Renders docs/demo.gif: an editor mock-up that replays a real riskmap session.
The terminal text comes from out1..3.txt, captured from the real binary."""
import sys
from PIL import Image, ImageDraw, ImageFont
here, out = sys.argv[1], sys.argv[2]
W, H = 920, 610
SIDE, TAB, SPLIT, LH = 190, 30, 368, 17
mono = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 13)
monob = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 13, index=1)
ui = ImageFont.truetype("/System/Library/Fonts/Helvetica.ttc", 12)
BG, SBG, TBG, LINE = "#1e1e2e", "#181825", "#11111b", "#313244"
FG, DIM, GREEN, RED, YEL, BLUE, MAUVE = "#cdd6f4", "#6c7086", "#a6e3a1", "#f38ba8", "#f9e2af", "#89b4fa", "#cba6f7"

config = open(f"{here}/repo/.riskmap.yml").read().rstrip("\n").split("\n")
button = open(f"{here}/repo/src/components/Button.tsx").read().rstrip("\n").split("\n")
ci = open(f"{here}/repo/.github/workflows/ci.yml").read().rstrip("\n").split("\n")
outs = [open(f"{here}/out{i}.txt").read().rstrip("\n").split("\n") for i in (1, 2, 3)]

TREE = [("v .github/workflows", None), ("    ci.yml", ".github/workflows/ci.yml"), ("> docs", None), ("> prisma", None),
        ("v src", None), ("  > app", None), ("  v components", None), ("      Button.tsx", "src/components/Button.tsx"),
        ("  > lib", None), ("  .riskmap.yml", ".riskmap.yml"), ("  package.json", None), ("  README.md", None)]

def yaml_color(line):
    s = line.strip()
    if s.startswith("#"): return DIM
    return FG

def draw_code(d, x, y, line, kind):
    if kind == "yaml" and ":" in line and not line.strip().startswith("#"):
        key, rest = line.split(":", 1)
        d.text((x, y), key + ":", font=mono, fill=BLUE)
        kx = x + d.textlength(key + ":", font=mono)
        if "#" in rest:
            val, com = rest.split("#", 1)
            d.text((kx, y), val, font=mono, fill=GREEN)
            d.text((kx + d.textlength(val, font=mono), y), "#" + com, font=mono, fill=DIM)
        else:
            d.text((kx, y), rest, font=mono, fill=GREEN)
    else:
        d.text((x, y), line, font=mono, fill=DIM if line.strip().startswith("#") else FG)

def frame(tab, lines, kind, term, changed=(), status=None, cursor=None, badges={}, caption=None):
    im = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(im)
    # sidebar
    d.rectangle((0, 0, SIDE, H), fill=SBG)
    d.text((14, 9), "EXPLORER", font=ui, fill=DIM)
    y = 34
    for label, path in TREE:
        active = path is not None and path.endswith(tab.split("/")[-1])
        if active: d.rectangle((0, y - 2, SIDE, y + 15), fill=LINE)
        col = FG if path else DIM
        if path in badges: col = {"U": GREEN, "M": YEL}[badges[path]]
        d.text((12, y), label, font=mono, fill=col)
        if path in badges: d.text((SIDE - 20, y), badges[path], font=mono, fill=col)
        y += 19
    # tab bar
    d.rectangle((SIDE, 0, W, TAB), fill=SBG)
    tw = d.textlength(tab.split("/")[-1], font=ui) + 28
    d.rectangle((SIDE, 0, SIDE + tw, TAB), fill=BG)
    d.text((SIDE + 14, 8), tab.split("/")[-1], font=ui, fill=FG)
    # editor
    y = TAB + 8
    for i, line in enumerate(lines):
        if y > SPLIT - LH: break
        if i in changed:
            d.rectangle((SIDE, y - 1, W, y + LH - 2), fill="#26392f")
            d.rectangle((SIDE, y - 1, SIDE + 3, y + LH - 2), fill=GREEN)
        d.text((SIDE + 12, y), f"{i+1:>2}", font=mono, fill=DIM)
        draw_code(d, SIDE + 44, y, line, kind)
        if cursor == i:
            cx = SIDE + 44 + d.textlength(line, font=mono)
            d.rectangle((cx + 1, y, cx + 8, y + 14), fill=FG)
        y += LH
    # terminal
    d.rectangle((SIDE, SPLIT, W, H), fill=TBG)
    d.line((SIDE, SPLIT, W, SPLIT), fill=LINE)
    d.text((SIDE + 14, SPLIT + 8), "TERMINAL", font=ui, fill=FG)
    d.line((SIDE + 14, SPLIT + 25, SIDE + 70, SPLIT + 25), fill=BLUE)
    y = SPLIT + 36
    for text, col, bold in term:
        d.text((SIDE + 14, y), text, font=monob if bold else mono, fill=col)
        y += LH
    if caption:
        tw = d.textlength(caption, font=ui)
        d.rounded_rectangle((W - tw - 34, 38, W - 12, 64), 6, fill=MAUVE)
        d.text((W - tw - 23, 45), caption, font=ui, fill=TBG)
    return im

frames, durs = [], []
def add(im, ms): frames.append(im); durs.append(ms)

def prompt(cmd="", failed=False, cur=True):
    return [("$ " + cmd + ("▌" if cur else ""), RED if failed and not cmd else FG, False)]

def type_cmd(base, cmd, mk, failed=False):
    for n in range(0, len(cmd) + 1, 3):
        add(mk(base + prompt(cmd[:n], failed)), 60)
    add(mk(base + prompt(cmd, failed)), 350)

def colour(lines):
    res = []
    for l in lines:
        if "FAILED" in l: res.append((l, RED, True))
        elif l.startswith("  high"): res.append((l, RED, False))
        elif l.startswith("  medium"): res.append((l, YEL, False))
        elif "ok," in l or "is valid" in l: res.append((l, GREEN, False))
        elif l.startswith("risk:"): res.append((l, FG, True))
        else: res.append((l, DIM if l.startswith("     ") else FG, False))
    return res

# 1. type the config
for n in range(1, len(config) + 1):
    add(frame(".riskmap.yml", config[:n], "yaml", prompt(), cursor=n - 1, badges={".riskmap.yml": "U"},
              caption="1  Describe which files are risky"), 150 if config[n - 1].strip() else 70)
durs[-1] = 1400
U = {".riskmap.yml": "U"}
mk = lambda t: frame(".riskmap.yml", config, "yaml", t, badges=U, caption="2  Check the rule file")
type_cmd([], "riskmap validate --format text", mk)
t1 = [("$ riskmap validate --format text", FG, False)] + colour(outs[0])
add(mk(t1 + prompt()), 2200)

# 2. harmless change
chg = [i for i, l in enumerate(button) if "className" in l]
M1 = {"src/components/Button.tsx": "M"}
mk = lambda t: frame("src/components/Button.tsx", button, "ts", t, changed=chg, badges=M1, caption="3  Change a component: fine")
add(mk(prompt()), 1300)
cmd = "git add . && riskmap check --staged --format text"
type_cmd([], cmd, mk)
add(mk([("$ " + cmd, FG, False)] + colour(outs[1]) + prompt()), 3200)

# 3. risky change
chg = [i for i, l in enumerate(ci) if "|| true" in l]
M2 = dict(M1, **{".github/workflows/ci.yml": "M"})
mk = lambda t: frame(".github/workflows/ci.yml", ci, "yaml", t, changed=chg, badges=M2, caption="4  Touch the CI workflow: blocked")
add(mk(prompt()), 1600)
type_cmd([], cmd, mk)
add(mk([("$ " + cmd, FG, False)] + colour(outs[2]) + [("", FG, False), ("exit code 3", RED, False)] + prompt()), 6000)

pal = frames[-1].quantize(colors=64, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE)
q = [f.quantize(palette=pal, dither=Image.Dither.NONE) for f in frames]
q[0].save(out, save_all=True, append_images=q[1:], duration=durs, loop=0, optimize=True, disposal=1)
print(len(frames), sum(durs) / 1000, "s")
