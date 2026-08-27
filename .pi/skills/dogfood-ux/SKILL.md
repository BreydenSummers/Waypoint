# Dogfood UX pass

Use this skill **before declaring any user-facing Waypoint feature done.** Passing automated tests is
necessary but NOT sufficient — this project requires that the agent actually runs the app, uses the
feature as an operator would, and visually judges it.

This pass assumes the host can run Docker Compose and Playwright; if either is missing, treat the
run as blocked rather than substituting a mocked browser flow.

Reference: `../../docs/design-spec.md` and `../../docs/waypoint-mockup.html` (the intended look), and
`../../PRD.md` (verification section).

## When to run

- Any change to a view, workspace, the trail map, the guide's note, the journey log, or the report
  output.
- Any change that alters what an operator sees or the flow they move through.

## Procedure

1. **Launch the running app** (not just unit tests). Use the project's run command / `docker compose
   up` and confirm it's serving.
2. **Drive the real UI as an operator would** — click through the actual flow the feature is part of
   (e.g. capture an action → see it in the journey log → promote a finding). Prefer real browser
   automation (e.g. Playwright) so the interaction is genuine, not a mocked render.
3. **Capture screenshots** of the feature in context.
4. **Compare against the design spec / mockup:**
   - palette + "artifact" surfaces (map/parchment stay fixed-color; chrome themed),
   - the cosy/trail feel is present in chrome, not competing with work surfaces,
   - vocabulary is security-domain-correct (journey log = audit trail, etc.),
   - motion stays under ~1s and respects `prefers-reduced-motion`.
5. **Check BOTH light and dark mode.**
6. **Check the accessible fallback** — the map has a non-visual ordered waypoint list; focus rings
   and `aria-current` are present.

## Definition of done

Only after: the flow makes sense when actually used, the screenshots match the intended design in
both themes, and nothing reads as a cold dashboard. Record what you checked. If it looks or feels
wrong, it is not done — fix it before closing the feature.
