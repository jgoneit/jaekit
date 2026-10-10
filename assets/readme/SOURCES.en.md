# README image guide

[한국어](SOURCES.md)

The README uses supplied Jaekit branding alongside conceptual illustrations of the product's roles.

## Branding

| Use | Preview | Files |
| --- | --- | --- |
| Primary logo | <img src="brand/logo.svg" width="200" alt="Jaekit symbol and wordmark"> | [Original PNG](brand/logo.png) · [Display SVG](brand/logo.svg) |
| Wordmark | <img src="brand/wordmark.svg" width="160" alt="Jaekit wordmark"> | [Original PNG](brand/wordmark.png) · [Display SVG](brand/wordmark.svg) |
| Small symbol | <img src="brand/symbol.svg" width="40" alt="Jaekit symbol"> | [Original PNG](brand/symbol.png) · [Display SVG](brand/symbol.svg) |

The three supplied PNGs are preserved unchanged. Each display SVG embeds the original PNG bytes on a rounded white surface so the black artwork stays visible in dark mode. The original aspect ratio and transparent padding are preserved. The primary logo appears at the top of the README; the other variants are shown here for reference.

Rebuild the display SVGs with [build_brand.py](source/build_brand.py). It uses only the Python 3 standard library. Run it from the repository root; it does not modify the PNGs.

```bash
python3 assets/readme/source/build_brand.py
```

## Conceptual illustrations

The hero and usage flow are **conceptual illustrations** of Jaekit's roles and usage flow. They are not screenshots, execution results, or evidence of completion or passed checks. The password-link request in the README is also a fictional example.

| Image | 한국어 | English |
| --- | --- | --- |
| Hero | [hero.ko.png](hero.ko.png) | [hero.en.png](hero.en.png) |
| Usage flow | [usage-flow.ko.png](usage-flow.ko.png) | [usage-flow.en.png](usage-flow.en.png) |
| Scalable usage flow | [usage-flow.ko.svg](usage-flow.ko.svg) | [usage-flow.en.svg](usage-flow.en.svg) |

The README uses PNG for preview compatibility. Alt text and nearby prose explain the images without relying on the artwork.

## Illustration source and editing

The two hero images were imported from the supplied README kit. Its creation notes record that the Korean artwork was made with OpenAI image generation and then localized into English. The [image prompts](source/IMAGE-PROMPTS.md) preserve the copy and composition instructions. PNGs have no editable text layers, so review both languages' wording and legibility after editing.

The usage flow was authored as SVG. The [usage flow source guide](source/USAGE-FLOW.md) covers editable text SVGs, generator scripts, and rebuilding. The final SVGs use outlined text and need no installed fonts. Python 3 with fonttools and Node.js with sharp are needed **only to edit the artwork**, not to use Jaekit or run the standard repository checks. The typeface is Noto Sans CJK KR; its [SIL OFL 1.1 license](source/FONT-LICENSE.txt) is included, but the font binaries are not.

## Meaning to preserve

- Spec defines the goal and completion conditions, then stops. The user reviews the goal and **starts Seal in a separate message**.
- The Codex or Claude Code agent performs implementation, checks, and fixes.
- Resuming requires naming the same goal in a new conversation in the same project.
- Completion reports describe changes and check records. Do not illustrate unrun checks as passed or promise independent verification or automatic deployment.
- Update both languages' images and alt text together.

Follow the [usage guide](../../guides/USAGE.en.md) for product behavior.
