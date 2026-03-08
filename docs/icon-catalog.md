# grüt Icon & Asset Catalog

Complete catalog of every icon, image, and asset needed for distribution across all platforms.

## Brand Assets (Source)

| Asset | File | Size | Format |
|-------|------|------|--------|
| Full logo (light bg) | `logo.svg` | scalable | SVG |
| Full logo (dark bg) | `logo-light.svg` | scalable | SVG |
| Icon ü (all sizes) | `icon-{16,32,48,64,128,256,512,1024}.svg` | per name | SVG |
| Icon ü light variants | `icon-{size}-light.svg` | per name | SVG |
| Primary icon | `icon.svg` | 128×128 | SVG |
| Favicon | `favicon.svg` | 32×32 | SVG |
| Brand mark (dots only) | `icon-simple.svg` | 32×32 | SVG |

---

## Platform Requirements

### 1. GitHub

| Asset | Size | Format | Where |
|-------|------|--------|-------|
| Repository social preview | 1280×640 | PNG/JPG | Settings → Social preview |
| Organization avatar | 500×500 | PNG | Org settings |
| Release asset icon | 128×128 | PNG | Shown in release cards |
| README logo | any | SVG/PNG | Embedded in README.md ✅ |
| Sponsor/funding image | 600×315 | PNG | `.github/FUNDING.yml` |

### 2. macOS (.app / Homebrew Cask)

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| App icon | 16×16 → 1024×1024 | ICNS (multi-res) | Required sizes: 16, 32, 64, 128, 256, 512, 1024 |
| Retina variants | @2x for each | ICNS | 32@2x=64px, 128@2x=256px, etc. |
| DMG background | 660×400 | PNG | If distributing .dmg |
| Menu bar icon | 22×22 | PNG (template) | If adding menu bar presence |

**ICNS generation:** Use `iconutil` on macOS or `png2icns`:
```bash
mkdir icon.iconset
cp icon-16.png icon.iconset/icon_16x16.png
cp icon-32.png icon.iconset/icon_16x16@2x.png
# ... etc for all sizes
iconutil -c icns icon.iconset
```

### 3. Windows (.exe / Scoop / Winget / Chocolatey)

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| Application icon | 16→256 multi-res | ICO | Sizes: 16, 24, 32, 48, 64, 128, 256 |
| Installer banner | 493×58 | BMP | If using WiX/MSI installer |
| Installer dialog | 493×312 | BMP | If using WiX/MSI installer |
| Taskbar icon | 32×32 or 48×48 | ICO | Extracted from app .ico |
| Start menu tile | 150×150 | PNG | UWP/MSIX packages |
| Wide tile | 310×150 | PNG | UWP/MSIX packages |

**ICO generation:**
```bash
# Using ImageMagick
convert icon-16.png icon-24.png icon-32.png icon-48.png icon-64.png icon-128.png icon-256.png grut.ico
```

**Package manager manifests:**
- **Scoop:** No icon field in manifest (uses .exe embedded icon)
- **Winget:** `Icons` field in manifest YAML (URL to icon hosted online)
- **Chocolatey:** `iconUrl` in .nuspec (URL to 48×48 or 64×64 PNG)

### 4. Linux (.deb / .rpm / Snap / Flatpak / AUR / AppImage)

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| Desktop entry icon | 16, 22, 24, 32, 48, 64, 128, 256, 512 | PNG | `/usr/share/icons/hicolor/{size}x{size}/apps/grut.png` |
| Scalable icon | any | SVG | `/usr/share/icons/hicolor/scalable/apps/grut.svg` |
| Snap store icon | 256×256 (min 40×40) | PNG | `snap/gui/grut.png` |
| Snap store banner | 1920×480 to 3840×960 | PNG | Store listing |
| Snap store screenshot | 480×854 min | PNG | Store listing |
| Flatpak/Flathub icon | 128×128 (min 64×64) | PNG/SVG | In Flatpak metadata |
| Flathub screenshot | 1600×900 | PNG | App listing |
| AppImage | 256×256 | PNG | Embedded in AppImage |
| .deb package | 32×32 | XPM/PNG | In `debian/` directory |

**Desktop entry file (`grut.desktop`):**
```ini
[Desktop Entry]
Name=grüt
Comment=AI-native terminal file explorer
Exec=grut
Icon=grut
Terminal=true
Type=Application
Categories=System;TerminalEmulator;Development;
```

### 5. Go Ecosystem (pkg.go.dev)

| Asset | Where | Notes |
|-------|-------|-------|
| Module logo | README.md | pkg.go.dev renders the README; logo shows via `<img>` tag ✅ |
| No separate icon upload | — | pkg.go.dev uses GitHub avatar for the org/user |

### 6. Docker Hub / Container Registries

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| Docker Hub org logo | 120×120 | PNG/JPG | Organization/user profile |
| Full description image | any | PNG | In `README.md` shown on Docker Hub |

### 7. VS Code Marketplace (if extension)

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| Extension icon | 128×128 | PNG | In `package.json` → `icon` field |
| Banner | 1280×640 | PNG | Marketplace listing |
| Gallery banner | 760×100 | PNG | Theme color in `package.json` |

### 8. Terminal / CLI Specific

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| ASCII art logo | — | Text | For `--version` / splash screen |
| Nerd Font icon | — | Unicode | Can use existing Nerd Font glyphs or custom PUA |
| Terminal color scheme | — | TOML/JSON | Malachite green accent `#0BDA51` |

### 9. Web / PWA (if web version)

| Asset | Size | Format | Notes |
|-------|------|--------|-------|
| favicon.ico | 16×16, 32×32 | ICO | Multi-res favicon |
| favicon.svg | scalable | SVG | Modern browsers ✅ |
| apple-touch-icon | 180×180 | PNG | iOS home screen |
| PWA icons | 192×192, 512×512 | PNG | `manifest.json` |
| PWA maskable | 512×512 (safe zone) | PNG | Adaptive icon with padding |
| Open Graph image | 1200×630 | PNG | Social sharing previews |
| Twitter card | 1200×628 | PNG | Twitter/X previews |

### 10. npm (if JS wrapper published)

| Asset | Where | Notes |
|-------|-------|-------|
| No icon in package | — | npm shows README from GitHub |
| npmjs.com avatar | — | Uses GitHub org/user avatar |

---

## Generation Pipeline

### SVG → PNG conversion
```bash
# Using Inkscape (recommended for quality)
inkscape icon.svg -w 128 -h 128 -o icon-128.png

# Using rsvg-convert (fast, headless)
rsvg-convert -w 128 -h 128 icon.svg > icon-128.png

# Using ImageMagick (widely available)
magick convert -background none -resize 128x128 icon.svg icon-128.png
```

### Generate all PNG sizes from SVG
```bash
for size in 16 22 24 32 48 64 128 256 512 1024; do
    rsvg-convert -w $size -h $size icon.svg > icon-${size}.png
done
```

### Generate platform-specific formats
```bash
# ICO (Windows) — requires ImageMagick
magick convert icon-16.png icon-24.png icon-32.png icon-48.png icon-64.png icon-128.png icon-256.png grut.ico

# ICNS (macOS) — requires iconutil on macOS
mkdir -p grut.iconset
for size in 16 32 64 128 256 512 1024; do
    cp icon-${size}.png grut.iconset/icon_${size}x${size}.png
done
iconutil -c icns grut.iconset -o grut.icns
```

### Social preview image (1280×640)
Create a centered logo on a dark background:
```bash
magick convert -size 1280x640 xc:'#1e1e2e' \
    \( logo-light.svg -resize 600x \) -gravity center -composite \
    social-preview.png
```

---

## Summary: What to Generate

| Priority | Asset | Needed For |
|----------|-------|------------|
| ✅ Done | SVG logo (dark/light) | README, web, docs |
| ✅ Done | SVG icons (all sizes) | Source for all rasterized formats |
| ✅ Done | SVG favicon | Web |
| 🔲 TODO | PNG exports (all sizes) | All platforms |
| 🔲 TODO | .ico (Windows) | Windows exe, installers |
| 🔲 TODO | .icns (macOS) | macOS app bundle |
| 🔲 TODO | Social preview (1280×640) | GitHub repo |
| 🔲 TODO | ASCII art | Terminal `--version` output |
| 🔲 TODO | .desktop file | Linux desktop integration |
| 🔲 TODO | Open Graph image | Social sharing |
