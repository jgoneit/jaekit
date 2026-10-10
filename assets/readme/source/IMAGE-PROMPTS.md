# Image generation prompts

These are the source prompts supplied with the README kit for the retained hero PNGs. The accompanying creation notes identify OpenAI image generation as the tool used. The usage flow is authored separately as SVG; see [USAGE-FLOW.md](USAGE-FLOW.md). These prompts describe conceptual product illustrations, not executed or verified examples. Editing an existing illustration should preserve its layout and update the matching language partner.

## Hero — Korean

Mode: new image. Output: `../hero.ko.png`.

```text
Use case: ads-marketing.
Create a finished, exceptionally clean flat 2D brand illustration/banner for the GitHub README of Jaekit, a tool used WITH Codex and Claude Code. This is the actual standalone image asset, not a mockup of a browser or a README page. Wide landscape, about 2.2:1, high resolution. It should make the purpose understandable in five seconds.

Art direction: sophisticated Swiss editorial typography meets friendly technical illustration. White/off-white background (#FAFBFD), near-black navy (#122033), vivid cobalt (#4F6CF7), one muted teal accent (#168975). Flat vector-like forms, crisp edges, generous negative space, highly controlled visual hierarchy. No 3D, no isometric objects, no gradients, no glassmorphism, no shadows, no robot/brain/sparkle/circuit clichés. Do not turn the whole image into a grid of boxed cards. Simple, memorable, intentional composition.

Left 55%: large wordmark "Jaekit" in a bold contemporary sans-serif near the top. Beneath it, a very large Korean headline on exactly two lines:
"AI 코딩 작업,"
"목표부터 완료 근거까지."
Small but readable secondary line:
"Codex · Claude Code와 함께"

Right 45%: an expressive but simple two-part illustration of a coding task becoming a clear document and a recorded result. Draw two large overlapping flat document sheets, clearly distinct:
1) cobalt-accented sheet titled "Spec", with three empty outline checkboxes and short neutral horizontal rules, a small drafting cursor. Under its title place exactly "목표 정리".
2) teal-accented sheet titled "Seal", with a simple code-bracket symbol and a tidy list of recorded lines, a small outlined checkmark emblem. Under its title place exactly "구현 · 검사 · 기록".
These are conceptual symbols, not screenshots or actual test outputs. Avoid fake code, fake log rows, numeric success claims or terminal output. The sheets can connect through one clean, graceful ribbon-like line which visually holds the composition together, but do not create a step-by-step flowchart or imply automatic execution after Spec.

Bottom aligned caption, roomy and readable:
"작업은 에이전트가. 목표와 근거는 기록으로."
Only the exact text provided above may appear. Korean lettering must be perfectly legible, with correct spacing and no overlapping. Make the image feel designed for a serious modern open-source developer tool, not a corporate presentation template.
```

## Hero — English

Mode: localization edit. Reference: `../hero.ko.png`. Output: `../hero.en.png`.

```text
Use case: text-localization
Asset type: English GitHub README hero banner for Jaekit.
Edit the provided Korean banner into its English counterpart. Preserve the same wide aspect ratio, composition, flat document artwork, navy Jaekit wordmark, cobalt Spec sheet, teal Seal sheet, connecting ribbon, whitespace, pale near-white background, and high-contrast modern sans-serif hierarchy. Replace EVERY Korean text label with English. Keep the artwork and existing brand spelling "Jaekit", "Spec", and "Seal". Do not invent new functionality or additional text.
Exact replacement text:
- Main headline on the left, two or three well-balanced lines: "AI coding tasks," then "from goals to evidence."
- Smaller left subtitle: "With Codex · Claude Code"
- Blue Spec document secondary label: "Define the goal"
- Teal Seal document secondary label: "Build · Check · Record"
- Bottom footer: "Agents do the work. Goals and evidence stay on record."
Keep all text fully inside the canvas, spelled exactly, and large enough to read when the banner is displayed at 850 pixels wide. English may reflow within the existing text areas to fit, without overlapping the illustration. Match the original type weights and restrained palette. No Korean characters should remain. No 3D additions, glossy effects, fake screenshot, new logo, watermark, statistics or fake verification results. Opaque near-white background.
```
