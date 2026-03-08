"""Generate grut icon variants using 0xProto Bold — ü character with Malachite dots."""
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.recordingPen import RecordingPen
import os

fonts_dir = r'D:\code\grut\assets\fonts\extracted'
output_dir = r'D:\code\grut\assets'

font_path = os.path.join(fonts_dir, '0xProto', '0xProtoNerdFontMono-Bold.ttf')
font = TTFont(font_path)
cmap = font.getBestCmap()
glyphset = font.getGlyphSet()
ascent = font['OS/2'].sTypoAscender
units_per_em = font['head'].unitsPerEm

DOT_COLOR = '#0BDA51'  # Malachite
TEXT_COLOR = '#2D2D2D'

# --- Extract 'u' glyph path ---
u_glyph_name = cmap[ord('u')]
pen = SVGPathPen(glyphset)
glyphset[u_glyph_name].draw(pen)
u_path = pen.getCommands()
u_advance = glyphset[u_glyph_name].width

# Get u glyph bounds
bounds_pen = BoundsPen(glyphset)
glyphset[u_glyph_name].draw(bounds_pen)
u_bounds = bounds_pen.bounds  # (xMin, yMin, xMax, yMax)

# --- Extract dot positions from udieresis glyph ---
udieresis_name = cmap[ord('\u00fc')]
rec = RecordingPen()
glyphset[udieresis_name].draw(rec)
contours = []
current = []
for op, args in rec.value:
    if op == 'moveTo':
        if current:
            contours.append(current)
        current = [args[0]]
    elif op in ('lineTo', 'qCurveTo', 'curveTo'):
        current.extend(args)
    elif op in ('closePath', 'endPath'):
        if current:
            contours.append(current)
        current = []
if current:
    contours.append(current)

contour_info = []
for c in contours:
    xs = [p[0] for p in c]
    ys = [p[1] for p in c]
    w = max(xs) - min(xs)
    h = max(ys) - min(ys)
    cx = (min(xs) + max(xs)) / 2
    cy = (min(ys) + max(ys)) / 2
    contour_info.append({'cx': cx, 'cy': cy, 'w': w, 'h': h, 'area': w * h})
contour_info.sort(key=lambda c: c['area'])
dot_contours = sorted(contour_info[:2], key=lambda c: c['cx'])
dot_left_x = dot_contours[0]['cx']
dot_right_x = dot_contours[1]['cx']
dot_y_font = dot_contours[0]['cy']
dot_font_radius = dot_contours[0]['w'] / 2

print(f"u bounds: {u_bounds}")
print(f"u advance: {u_advance}")
print(f"Dot positions: left_x={dot_left_x:.0f}, right_x={dot_right_x:.0f}, y={dot_y_font:.0f}, r={dot_font_radius:.0f}")

# --- Generate icon at various sizes ---
sizes = [16, 32, 48, 64, 128, 256, 512, 1024]

for size in sizes:
    padding = size * 0.1
    usable = size - 2 * padding

    # Scale to fit the full ü (including dots) in the usable area
    glyph_height_font = dot_y_font + dot_font_radius - u_bounds[1]  # bottom of u to top of dots
    glyph_width_font = u_bounds[2] - u_bounds[0]

    scale_h = usable / glyph_height_font
    scale_w = usable / glyph_width_font
    scale = min(scale_h, scale_w)

    # Center horizontally
    rendered_width = glyph_width_font * scale
    x_offset = (size - rendered_width) / 2 - u_bounds[0] * scale

    # Baseline position: bottom of u at padding + rendered_glyph_below_baseline
    rendered_height = glyph_height_font * scale
    y_start = (size - rendered_height) / 2
    baseline_y = y_start + (dot_y_font + dot_font_radius) * scale

    dot_radius = dot_font_radius * scale * 1.2  # slightly bigger dots

    # Dot positions in SVG coords
    dot_left_svg_x = x_offset + dot_left_x * scale
    dot_right_svg_x = x_offset + dot_right_x * scale
    dot_svg_y = baseline_y - dot_y_font * scale

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" width="{size}" height="{size}">',
        f'  <!-- grut icon - u with Malachite umlaut ({size}x{size}) -->',
        f'  <g transform="translate({x_offset:.2f}, {baseline_y:.2f}) scale({scale:.6f}, -{scale:.6f})">',
        f'    <path d="{u_path}" fill="{TEXT_COLOR}"/>',
        f'  </g>',
        f'  <circle cx="{dot_left_svg_x:.2f}" cy="{dot_svg_y:.2f}" r="{dot_radius:.2f}" fill="{DOT_COLOR}"/>',
        f'  <circle cx="{dot_right_svg_x:.2f}" cy="{dot_svg_y:.2f}" r="{dot_radius:.2f}" fill="{DOT_COLOR}"/>',
    ]

    # Add pulse animation for sizes >= 64
    if size >= 64:
        for cx in [dot_left_svg_x, dot_right_svg_x]:
            parts.append(
                f'  <circle cx="{cx:.2f}" cy="{dot_svg_y:.2f}" r="{dot_radius:.2f}" '
                f'fill="{DOT_COLOR}" opacity="0">'
            )
            parts.append(
                f'    <animate attributeName="r" values="{dot_radius:.2f};{dot_radius + size * 0.02:.2f};{dot_radius:.2f}" '
                f'dur="2s" repeatCount="indefinite"/>'
            )
            parts.append(
                f'    <animate attributeName="opacity" values="0.4;0;0.4" '
                f'dur="2s" repeatCount="indefinite"/>'
            )
            parts.append('  </circle>')

    parts.append('</svg>')

    out_path = os.path.join(output_dir, f'icon-{size}.svg')
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(parts))
    print(f'OK: icon-{size}.svg')

# Also create the main icon.svg (128px) and favicon.svg (32px)
import shutil
shutil.copy2(os.path.join(output_dir, 'icon-128.svg'), os.path.join(output_dir, 'icon.svg'))
shutil.copy2(os.path.join(output_dir, 'icon-32.svg'), os.path.join(output_dir, 'favicon.svg'))
print('Copied icon-128 -> icon.svg, icon-32 -> favicon.svg')

# Create icon-simple.svg — just the two dots (brand mark), 32x32
simple = [
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">',
    '  <!-- grut brand mark - two Malachite dots -->',
    f'  <circle cx="11" cy="16" r="5" fill="{DOT_COLOR}"/>',
    f'  <circle cx="21" cy="16" r="5" fill="{DOT_COLOR}"/>',
    '</svg>'
]
with open(os.path.join(output_dir, 'icon-simple.svg'), 'w') as f:
    f.write('\n'.join(simple))
print('OK: icon-simple.svg')

# Create white variants for dark backgrounds
for sz in sizes:
    src = os.path.join(output_dir, f'icon-{sz}.svg')
    dst = os.path.join(output_dir, f'icon-{sz}-light.svg')
    with open(src, 'r') as f:
        content = f.read()
    content = content.replace(f'fill="{TEXT_COLOR}"', 'fill="#FFFFFF"')
    with open(dst, 'w') as f:
        f.write(content)

# Main light icon
shutil.copy2(os.path.join(output_dir, 'icon-128-light.svg'), os.path.join(output_dir, 'icon-light.svg'))
print('Created all light icon variants')

font.close()
print('Done!')
