# Sentinel Soft — Design System

## Overview

Sentinel Soft is a minimal, comfortable design language for the Pocket Sentinel dashboard. It prioritises readability, gentle contrast, and clean structure in both dark and light modes. The system uses rounded surfaces, subtle borders, and restrained color to let data breathe — no decoration for decoration's sake.

---

## Colors

### Dark Mode (Default)
| Token | Value | Usage |
|---|---|---|
| `--canvas` | `#1a1b23` | Page & card background |
| `--parchment` | `#13141a` | Body / shell background |
| `--pearl` | `#22232d` | Elevated surface, input bg |
| `--dark` | `#13141a` | Alternate section bg |
| `--dark-2` | `#1e1f28` | Condition cards |
| `--ink` | `#e4e4e9` | Primary text |
| `--muted` | `#8b8b96` | Secondary / caption text |
| `--muted-dark` | `#9d9da8` | Tertiary text |
| `--hairline` | `#2e303c` | Borders |
| `--hairline-dark` | `#3c3e4c` | Heavier borders |
| `--border-subtle` | `#2e303c` | Subtle dividers |
| `--bg-hover` | `rgba(255,255,255,0.06)` | Hover tint |
| `--card` | `#1a1b23` | Card surface |

### Light Mode
| Token | Value | Usage |
|---|---|---|
| `--canvas` | `#ffffff` | Page & card background |
| `--parchment` | `#f6f6f9` | Body / shell background |
| `--pearl` | `#f0f0f5` | Elevated surface, input bg |
| `--ink` | `#2c2c31` | Primary text |
| `--muted` | `#71717a` | Secondary text |
| `--hairline` | `#e4e4ea` | Borders |
| `--card` | `#ffffff` | Card surface |

### Accent & Semantic
| Token | Dark | Light | Usage |
|---|---|---|---|
| `--blue` | `#2997ff` | `#0066cc` | Interactive, links |
| `--blue-focus` | `#0071e3` | `#0071e3` | Focus rings |
| `--green` | `#30d158` | `#248a3d` | Success, healthy |
| `--amber` | `#ffd60a` | `#b45f06` | Warning |
| `--red` | `#ff453a` | `#d70015` | Error, critical |

---

## Typography

- **System Font Stack:** `-apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", system-ui, sans-serif`
- **Mono:** `ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace`

| Element | Size | Weight | Line-height |
|---|---|---|---|
| h1 | `clamp(32px, 5vw, 44px)` | 600 | 1.08 |
| h2 | 22px | 600 | 1.2 |
| h3 | 15–16px | 600 | 1.3 |
| body | 17px | 400 | 1.45 |
| caption / label | 11–13px | 500–600 | 1.35 |
| mono / data | 12–13px | 500 | 1.4 |

---

## Spacing

Base unit: **4px**

| Token | Value |
|---|---|
| xs | 4px |
| sm | 8px |
| md | 12px |
| lg | 16px |
| xl | 24px |
| 2xl | 32px |
| 3xl | 48px |

---

## Border Radius

| Token | Value | Usage |
|---|---|---|
| `--radius` | 18px | Cards, sections, modals |
| small | 10–12px | Buttons, inputs, pills, conditions |
| pill | 999px | Badges, toggles, sync pills |
| circle | 50% | Avatars, dots |

All interactive and container elements use rounded corners. No sharp / 0px radius.

---

## Elevation

Shadows are minimal and serve structural hierarchy, not decoration.

| Level | Value | Usage |
|---|---|---|
| card | `0 2px 8px rgba(0,0,0,0.06)` | Default cards |
| elevated | `0 4px 16px rgba(0,0,0,0.10)` | Modals, drawers |
| pressed | (none) | Active / pressed states |

---

## Components

### Buttons
- **Primary:** `--blue` background, white text, pill radius, no border. Hover: slight opacity reduction.
- **Secondary:** Transparent, `--ink` text, 1px `--hairline` border, pill radius. Hover: `--bg-hover` fill.
- **Ghost:** No border, `--blue` text. Hover: underline.
- **Disabled:** 60% opacity, `cursor: not-allowed`.
- **Min tap size:** 44px height.

### Cards
- Background: `var(--canvas)`. Border: 1px `var(--hairline)`. Radius: `var(--radius)`.
- Shadow: card-level. Padding: 20px.

### Metric Cards
- Same as card. Label: uppercase mono, `--muted`. Value: large weight-600 `--ink`.
- Progress meters: 5px tall, rounded, `--bg-hover` track.

### Navigation (Bottom Tab Bar)
- Fixed bottom, blurred background. 4-column icon grid.
- Each tab: icon (Remix Icon) + Thai label. Active: `--blue` tint.
- No numbered marks — icons only.

### Theme Toggle
- Circular icon button (34px). Sun/moon Remix Icon.
- No text label — icon communicates mode.

### Inputs & Textareas
- Background: `var(--pearl)`. Border: 1px `var(--hairline)`. Radius: 12px.
- Focus: `--blue` border + subtle blue ring.

### Badges & Pills
- Pill shape (999px). Semantic-colored background at 15–18% opacity with matching text.

---

## Icons

The dashboard uses **Remix Icon** (`remixicon@4.5.0`) via CDN. Prefer line-weight variants (`ri-*-line`) for consistency. Use filled variants (`ri-*-fill`) only for active / selected states.

Key icon mappings:
| Context | Icon |
|---|---|
| Theme: dark → light | `ri-sun-line` |
| Theme: light → dark | `ri-moon-line` |
| Nav: Overview | `ri-dashboard-line` |
| Nav: Schedule | `ri-calendar-line` |
| Nav: Map | `ri-map-pin-line` |
| Nav: Telegram | `ri-send-plane-line` |
| Refresh | `ri-refresh-line` |

---

## Do's and Don'ts

1. **Do** keep contrast gentle — avoid pure #000 on #fff or #fff on #000.
2. **Do** use `var(--radius)` for all cards and sections; smaller radius for nested elements.
3. **Do** use icons from Remix Icon for navigation and toggles.
4. **Don't** add decorative gradients, glassmorphism, or neon glow.
5. **Don't** use thick borders (>1px) as the primary visual organiser.
6. **Do** let spacing and typography create hierarchy — not color saturation or border weight.
7. **Do** test both themes; neither should feel harsh or hard to read.
8. **Don't** add new fonts or heavy typefaces — system fonts are sufficient.
