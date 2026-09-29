# Kryton social cards

Static HTML sources for the images used in link previews (Slack, GitHub, X,
Open Graph). Rendered with headless Chrome and converted with macOS `sips` —
see [`build-social-card.sh`](build-social-card.sh).

| File | Size | Used for |
| --- | --- | --- |
| `kryton-share-card.html` → `kryton-share-card.png` | 1200×630 | README hero / Open Graph share card |
| `kryton-social-card.html` → `kryton-social-card.jpg` | 1600×900 | Wide social card (X/Twitter, LinkedIn) |
| `zyvor-mark.svg` | — | Zyvor "Z" mark used in both cards' top-left lockup |

Regenerate after any edit:

```sh
./docs/social/build-social-card.sh
```

Every claim shown on the cards (Stable UUIDs, `demo` → `dockur` → `kubevirt`,
KubeVirt = GA path, `dockur` lab installer not GA, Go 1.23+, Apache-2.0, etc.)
is sourced from the project's top-level `README.md` and the rest of `docs/`.
If you change wording on a card, verify it against those sources — don't
invent new claims.

## Palette

The cards share the same Apple.com-style system as the MkDocs site
(`docs/stylesheets/apple-glass.css`), so link previews match the docs site
instead of introducing a separate brand:

- **Background / surfaces:** white `#ffffff`, hairline `#d2d2d7`.
- **Text:** ink `#1d1d1f` (headings/body), secondary `#6e6e73` (subheads,
  footers).
- **Brand blue (dominant color):** gradient `#0071e3` → `#2997ff` — used for
  the accent bar, the Zyvor mark, the emphasized headline phrase, chips, and
  step/lane accents.
- **Orange accent `#ff6a2a`:** used exactly once per image, as a small dot
  next to the footer's version line — never as a background or primary
  color.
- **Zyvor "Z" mark:** `zyvor-mark.svg`, blue gradient (`#0071e3` →
  `#2997ff`), matching the brand blue above. This is a dedicated copy for
  the social cards; it does not affect the sitewide logo/favicon in
  `docs/assets/readme/zyvor-logo.svg`, which still uses the legacy mark.
- **Type:** Helvetica Neue (sans) for copy, SF Mono/Menlo (mono) for
  labels/footers — unchanged.

Previously the cards used a near-black/warm-cream/orange palette
(`--ink:#0d0d0c`, `--soft:#5a544c`, `--signal:#ff5a15`, `--deep:#cc420a`,
`--warm:#fbf7f2`, plus stray green/blue chip colors on the 16:9 card). That
has been fully replaced by the palette above.
