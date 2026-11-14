#!/usr/bin/env python3
"""Generate json5 hero visual (GitHub dark palette, no-cringe flat design)."""
from PIL import Image, ImageDraw, ImageFont

BG = (13, 17, 23)
FG = (230, 237, 243)
MUTED = (139, 148, 158)
GO = (0, 173, 216)
ACCENT = (247, 223, 30)   # JS yellow
BORDER = (48, 54, 61)

SANS = "/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf"
SANS_R = "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf"
MONO = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"


def hero():
    W, H = 1280, 640
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)
    for x in range(0, W, 64):
        d.line([x, 0, x, H], fill=(20, 25, 32))
    for y in range(0, H, 64):
        d.line([0, y, W, y], fill=(20, 25, 32))
    # flat code card on the right
    cx, cy, cw, ch = 780, 130, 400, 340
    d.rounded_rectangle([cx, cy, cx + cw, cy + ch], radius=18, outline=BORDER, width=4, fill=(22, 27, 34))
    code = ["{", "  // hi", "  a: 'x',", "  b: 0xdecaf,", "  c: .5,", "}"]
    for i, ln in enumerate(code):
        col = MUTED if ln.strip().startswith("//") else FG
        d.text((cx + 36, cy + 40 + i * 48), ln, font=ImageFont.truetype(MONO, 36), fill=col)
    d.text((90, 210), "json5", font=ImageFont.truetype(SANS, 120), fill=FG)
    d.text((94, 370), "JSON5 for Go. The whole spec.", font=ImageFont.truetype(SANS_R, 44), fill=MUTED)
    d.text((94, 500), "$ go get github.com/ofershap/json5", font=ImageFont.truetype(MONO, 30), fill=GO)
    img.save("assets/hero.png")


hero()
