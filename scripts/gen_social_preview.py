from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


WIDTH = 1280
HEIGHT = 640
OUT = Path(".github/assets/social-preview.png")


def load_font(size: int, bold: bool = False) -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    candidates = [
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf" if bold else "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
        "/usr/share/fonts/dejavu/DejaVuSans-Bold.ttf" if bold else "/usr/share/fonts/dejavu/DejaVuSans.ttf",
    ]
    for path in candidates:
        p = Path(path)
        if p.exists():
            return ImageFont.truetype(str(p), size=size)
    return ImageFont.load_default()


def vertical_gradient(size: tuple[int, int], top: tuple[int, int, int], bottom: tuple[int, int, int]) -> Image.Image:
    img = Image.new("RGB", size, top)
    draw = ImageDraw.Draw(img)
    for y in range(size[1]):
        t = y / max(size[1] - 1, 1)
        color = tuple(int(top[i] * (1 - t) + bottom[i] * t) for i in range(3))
        draw.line((0, y, size[0], y), fill=color)
    return img


def rounded_box(draw: ImageDraw.ImageDraw, xy: tuple[int, int, int, int], fill, outline, width: int, radius: int) -> None:
    draw.rounded_rectangle(xy, radius=radius, fill=fill, outline=outline, width=width)


def main() -> None:
    img = vertical_gradient((WIDTH, HEIGHT), (12, 18, 29), (8, 42, 48))
    draw = ImageDraw.Draw(img)

    panel = (72, 72, 1208, 568)
    rounded_box(draw, panel, fill=(20, 27, 41), outline=(61, 227, 204), width=3, radius=26)
    draw.line((104, 216, 1176, 216), fill=(59, 78, 103), width=2)

    title_font = load_font(92, bold=True)
    subtitle_font = load_font(40, bold=True)
    body_font = load_font(28, bold=False)
    chip_font = load_font(28, bold=True)

    draw.text((104, 106), "YrestAPI", font=title_font, fill=(244, 247, 250))
    draw.text((104, 238), "YAML -> POSTGRES -> JSON", font=subtitle_font, fill=(113, 232, 212))

    chips = [
        ((104, 332, 332, 390), "READ-ONLY"),
        ((360, 332, 720, 390), "NESTED RELATIONS"),
        ((748, 332, 1118, 390), "FILTERS / SORTS"),
    ]
    for box, label in chips:
        rounded_box(draw, box, fill=(26, 40, 61), outline=(61, 227, 204), width=2, radius=16)
        text_bbox = draw.textbbox((0, 0), label, font=chip_font)
        text_w = text_bbox[2] - text_bbox[0]
        text_h = text_bbox[3] - text_bbox[1]
        x = box[0] + ((box[2] - box[0]) - text_w) / 2
        y = box[1] + ((box[3] - box[1]) - text_h) / 2 - 2
        draw.text((x, y), label, font=chip_font, fill=(244, 247, 250))

    draw.text((104, 452), "Declarative API layer for Postgres", font=body_font, fill=(173, 184, 201))
    draw.text((104, 494), "Configured in YAML, served as JSON", font=body_font, fill=(173, 184, 201))

    OUT.parent.mkdir(parents=True, exist_ok=True)
    img.save(OUT, format="PNG", optimize=True)


if __name__ == "__main__":
    main()
