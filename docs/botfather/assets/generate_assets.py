#!/usr/bin/env python3
"""Генерация ассетов для BotFather: description picture, botpic, анимированный GIF.

Запуск: PYTHONPATH=.sandbox-cache/pylibs python3 docs/botfather/assets/generate_assets.py
Требует Pillow. Палитра — спокойная («Ритм»): тёмно-зелёный, мята, песок.
"""
import math
import os

from PIL import Image, ImageDraw, ImageFilter, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
FONT_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
FONT_REGULAR = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"

BG_TOP = (11, 24, 20)
BG_BOTTOM = (22, 42, 35)
MINT = (126, 184, 160)
MINT_SOFT = (159, 198, 180)
TEXT = (232, 241, 235)
MUTED = (124, 158, 144)
SAND = (192, 166, 122)


def gradient(size, top, bottom):
    width, height = size
    image = Image.new("RGB", size)
    pixels = image.load()
    for y in range(height):
        t = y / max(1, height - 1)
        color = tuple(round(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        for x in range(width):
            pixels[x, y] = color
    return image


def add_glow(image, center, radius, color):
    layer = Image.new("RGBA", image.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    for step in range(6, 0, -1):
        alpha = 18 - step * 2
        r = radius * step / 6
        draw.ellipse(
            [center[0] - r, center[1] - r, center[0] + r, center[1] + r],
            fill=(*color, max(0, alpha)),
        )
    layer = layer.filter(ImageFilter.GaussianBlur(radius / 3))
    image.alpha_composite(layer)


def draw_wave(draw, width, base_y, amplitude, period, phase, color, line_width=2):
    points = []
    for x in range(0, width + 1, 3):
        y = base_y + amplitude * math.sin((x / period) * 2 * math.pi + phase)
        points.append((x, y))
    draw.line(points, fill=color, width=line_width, joint="curve")


def draw_leaf(draw, x, y, size, color):
    """Простой лист из двух кривых + стебель."""
    upper, lower = [], []
    for i in range(0, 101, 2):
        t = i / 100
        dx = size * t
        upper.append((x + dx, y - size * 0.55 * math.sin(math.pi * t)))
        lower.append((x + dx, y + size * 0.30 * math.sin(math.pi * t)))
    draw.polygon(upper + list(reversed(lower)), outline=color)
    draw.line([(x, y), (x - size * 0.12, y + size * 0.35)], fill=color, width=2)


def description_picture(path):
    size = (640, 360)
    base = gradient(size, BG_TOP, BG_BOTTOM).convert("RGBA")
    add_glow(base, (515, 80), 230, (60, 120, 95))

    layer = Image.new("RGBA", size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    draw_wave(draw, 640, 292, 16, 210, 0.3, (*MINT, 70), 2)
    draw_wave(draw, 640, 312, 24, 170, 1.1, (*MINT, 42), 1)
    draw_wave(draw, 640, 330, 10, 240, 2.0, (*SAND, 46), 1)
    draw_leaf(draw, 512, 96, 78, (*SAND, 190))
    base.alpha_composite(layer)

    draw = ImageDraw.Draw(base)
    draw.text((54, 72), "РИТМ", font=ImageFont.truetype(FONT_BOLD, 108), fill=TEXT)
    draw.line([(58, 196), (196, 196)], fill=(*MINT, 210), width=2)
    draw.text((56, 212), "слушать тело · держать ритм",
              font=ImageFont.truetype(FONT_REGULAR, 29), fill=MINT_SOFT)
    draw.text((58, 264), "утренний чек-ин · сон · HRV · пульс · питание",
              font=ImageFont.truetype(FONT_REGULAR, 17), fill=MUTED)
    base.convert("RGB").save(path, "PNG", optimize=True)


def botpic(path):
    size = (512, 512)
    base = gradient(size, (9, 20, 17), (26, 48, 40)).convert("RGBA")
    add_glow(base, (256, 190), 250, (60, 120, 95))

    layer = Image.new("RGBA", size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    draw_wave(draw, 512, 360, 18, 190, 0.8, (*MINT, 110), 4)
    draw_wave(draw, 512, 396, 26, 150, 1.9, (*MINT, 55), 2)
    draw_wave(draw, 512, 430, 12, 220, 2.7, (*SAND, 55), 2)
    draw_leaf(draw, 352, 128, 64, (*SAND, 200))
    base.alpha_composite(layer)

    draw = ImageDraw.Draw(base)
    font = ImageFont.truetype(FONT_BOLD, 250)
    bbox = draw.textbbox((0, 0), "Р", font=font)
    width = bbox[2] - bbox[0]
    height = bbox[3] - bbox[1]
    draw.text((256 - width / 2 - bbox[0], 96 - bbox[1]), "Р", font=font, fill=TEXT)
    base.convert("RGB").save(path, "PNG", optimize=True)


def description_gif(path, frames=24):
    size = (320, 180)
    rendered = []
    for frame_index in range(frames):
        phase = 2 * math.pi * frame_index / frames
        base = gradient(size, (11, 24, 20), (22, 42, 35)).convert("RGBA")
        layer = Image.new("RGBA", size, (0, 0, 0, 0))
        draw = ImageDraw.Draw(layer)
        draw_wave(draw, 320, 128, 9, 120, phase, (*MINT, 120), 2)
        draw_wave(draw, 320, 148, 13, 100, phase + 1.2, (*MINT, 60), 1)
        draw_wave(draw, 320, 164, 6, 140, phase + 2.2, (*SAND, 60), 1)
        base.alpha_composite(layer)

        draw = ImageDraw.Draw(base)
        draw.text((22, 28), "РИТМ", font=ImageFont.truetype(FONT_BOLD, 34), fill=TEXT)
        draw.text((24, 72), "слушать тело · держать ритм",
                  font=ImageFont.truetype(FONT_REGULAR, 13), fill=MINT_SOFT)
        rendered.append(base.convert("RGB"))
    rendered[0].save(path, "GIF", save_all=True, append_images=rendered[1:],
                     duration=120, loop=0, optimize=True)


if __name__ == "__main__":
    description_picture(os.path.join(HERE, "description.png"))
    description_gif(os.path.join(HERE, "description.gif"))
    botpic(os.path.join(HERE, "botpic.png"))
    print("assets written to", HERE)
