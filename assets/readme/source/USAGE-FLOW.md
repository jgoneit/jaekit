# Usage flow artwork

The usage diagram is an exact, native SVG illustration. It is not a screenshot,
an execution transcript, or a claim that any checks passed. The four stages are:

1. Ask Spec. It writes the goal, the completion conditions and an `Assurance:`
   tier, asks once whether to implement it now, and stops. Review the goal.
2. Answer yes and Seal starts. Naming the goal folder in a separate message
   starts it too.
3. The agent plans, implements and checks through `ha`, which records every
   run.
4. `ha done` records completion when the required conditions are met, and the
   report lands in `PROGRESS.md`.

A band below the stages shows independent review by tier: `light` may skip it
with a reason; `standard` gets Spar review rounds and a gate that must cover the
last commit; `strict` also gets a plan review before work. A panel beside the
tiers shows that Seal asks the user only about decisions only the user can make:
the foreseeable ones in one message before work, only new questions during work,
such as one that would change the requirements, and the confirmations in one
message at the end. Resuming requires opening the same project and explicitly
specifying the same goal in a new conversation. The illustration does not
promise automatic resume.

## Files

- `assets/readme/usage-flow.ko.svg` and `.en.svg`: outlined text, font independent.
- `assets/readme/usage-flow.ko.png` and `.en.png`: 1600 × 960 PNG exports.
- `assets/readme/source/svg/usage-flow.ko.text.svg` and `.en.text.svg`: editable text versions.
- `assets/readme/source/build_usage_flow.py`: layout, artwork and both languages' copy.
- `assets/readme/source/render_usage_flow.cjs`: renders these two PNGs.
- `assets/readme/source/usage-flow.layout.json`: measured text bounds for layout inspection.

The font, the shared drawing code, the rebuild commands and the other README
images are described in [ARTWORK.md](ARTWORK.md). To rebuild only the usage
flow, run from the repository root:

```bash
python3 assets/readme/source/build_usage_flow.py --font-dir /path/to/fonts
node assets/readme/source/render_usage_flow.cjs
```

These dependencies are optional artwork tools; they are not required to use
Jaekit or run the standard repository checks.
