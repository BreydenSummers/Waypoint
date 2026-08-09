# Waypoint — design spec

A staged-task tool themed as a guided journey through the woods. The user completes a multi-stage workflow; the interface presents it as an expedition along a trail, with the tool acting as a friendly guide. This document is the source of truth for the visual design, vocabulary, and interaction mechanics. A reference mockup lives at `waypoint-mockup.html`.

> **Note (adaptation for Waypoint):** this reference is *general* and modeled as a linear, locked-stage wizard. Waypoint's Recon / Attacks / Findings phases are worked continuously and in parallel, so adopt the aesthetic, vocabulary, and mechanics but re-map the gating: waypoints are always-accessible engagement phases/milestones, and "fog/locked" means "no data discovered here yet," not "not yet permitted." See `../PRD.md` → *Design & aesthetics* for the full adaptation and the security-domain vocabulary map.

## Core concept

- The **home view is an illustrated trail map**, not a menu or stepper. Stages are waypoints (camps/landmarks) placed along a winding dotted path through terrain.
- Clicking an unlocked waypoint opens that stage's **workspace** (the actual task UI: forms, editors, whatever the stage needs). The workspace stays clean and legible; the woodland atmosphere lives in the chrome, map, and transitions — never competing with form fields.
- The tool **speaks as a guide**: proactive, briefing the user on what's ahead, never robotic system language.

## Color palette (woody, warm)

| Role | Hex | Usage |
|---|---|---|
| Deep bark | #3B2617 | Darkest chrome, sidebar/header backgrounds |
| Bark | #4A2F1B | Tree silhouettes, dark panels |
| Walnut | #5C4033 | Secondary dark, tree variation |
| Saddle | #6B4423 | Primary buttons |
| Trail brown | #8B5E34 | The dotted trail stroke |
| Harvest | #BA7517 | Accent: icons, active borders, links |
| Lantern | #EF9F27 | Current-waypoint fill, highlights |
| Wheat | #FAC775 | Current-waypoint ring, light accent text on dark |
| Parchment | #FAEEDA | Light panels, guide's note background, badges |
| Map cream | #E8DCC3 | Map terrain background |
| Contour | #D4C4A0 | Topographic contour lines |
| Moss | #6B8E4E / #55703D / #8FA968 | Tree fills on the map |
| Fern | #97C459 | Completed waypoint fill |
| Forest | #639922 | Completed waypoint fill (map) |
| Pine text | #173404 | Text/icons on fern/forest |
| Dark cocoa text | #633806 | Headings on parchment |
| Cocoa text | #854F0B | Body text on parchment |
| Stone | #B4A78C / #8B7355 | Locked waypoints, signposts |

Rules: text on parchment is always cocoa/dark cocoa, never black or gray. Support dark mode by keeping the map and parchment panels as fixed-color "artifact" surfaces (they read as a physical map in both modes) while surrounding app chrome uses the framework's theme variables.

## Layout

1. **Map home (primary view)**
   - Full-width SVG trail map: cream terrain, contour lines, tree clusters, a lake, a mountain at the summit end.
   - Winding dotted trail (`stroke-dasharray`, round linecap) connecting 4–6 waypoints. The path visibly climbs toward the summit — harder/heavier stages sit higher.
   - Header: expedition name, day count, waypoints reached; stat chips for "Traveled" and "To summit" (themed distance instead of raw %).
   - Below the map: **Guide's note** panel (parchment) + **Journey log** panel side by side.

2. **Stage workspace (drill-in)**
   - Opens when the user enters a waypoint. Clean light surface, minimal brown chrome: a slim header with stage badge ("Stage 3 of 5 · The ridge"), autosave indicator ("Saved 2 min ago").
   - Footer nav: back = "Back to <previous waypoint>", forward = "Continue to <next waypoint>" (saddle brown primary button).
   - Optional slim sidebar variant: 220px deep-bark rail with the vertical dotted trail and mini waypoint list, tree-silhouette footer (see mockup).

## Waypoint states

| State | Visual | Behavior |
|---|---|---|
| Completed | Fern/forest green circle, white check | Clickable — user can revisit and edit |
| Current | Larger lantern-orange circle, wheat ring, campfire icon, "you are here" label | The active stage |
| Locked | Stone circle, signpost or lock icon, ~55% opacity ("fog") | Not clickable; tooltip explains what unlocks it |
| Needs attention (optional) | Amber ring + small flag | Stage completed but flagged for review |

## Guide mechanics (the "helping along" part)

- **Guide's note**: always one waypoint ahead. Briefs the user on: what the next stage involves, what they already have ready ("Your imported records are in your pack"), and a typical time estimate ("Most travelers cross in about 10 minutes"). One primary CTA: "Continue up the ridge".
- **Journey log**: auto-written trip diary, one entry per meaningful action ("Day 2 — Creek crossed. 240 records packed."). Doubles as an audit/activity trail. Current entry highlighted in harvest.
- **Pack / provisions**: accumulated inputs (uploads, configs, decisions) that persist across stages and are referenced by later stages.
- **Errors as guide advice**: "Can't break camp yet — you'll need a rule name in your pack." Never "Error: field required."
- **Completion**: reaching the summit produces a shareable **trip summary** — the finished map with route drawn solid, log, stats, summit flag.

## Vocabulary map

| Generic | Waypoint term |
|---|---|
| Step / stage | Waypoint (named: Trailhead, Creek crossing, The ridge, Lookout, Summit) |
| Start | Break camp |
| Save & exit | Make camp |
| Progress % | Distance traveled / distance to summit |
| Wizard complete | Reach the summit |
| Activity history | Journey log |
| Saved inputs | Your pack |
| Blocked | Fog on the trail |
| Help panel | Guide's note |

## Motion & transitions

- On stage completion: the dotted trail **animates drawing** from the finished waypoint to the next (SVG `stroke-dashoffset` animation, ~800ms ease-out), then the next waypoint "lights up."
- Current waypoint gets a subtle campfire flicker (gentle scale/opacity pulse, respect `prefers-reduced-motion`).
- Optional: slow parallax on layered tree silhouettes in the header/hero (Firewatch-style, subtle).
- No scroll hijacking. Keep animations under 1s; the tool must feel fast.

## Accessibility & practical notes

- The map must have a non-visual equivalent: an ordered list of waypoints with state labels (visually hidden or as an alternate view toggle).
- Waypoints are real buttons/links with focus states (harvest focus ring), labels, and aria-current="step" on the active one.
- Minimum text size 12px on the map labels, 13px elsewhere; cocoa-on-parchment and wheat-on-bark pairs are the approved contrast combos.
- Mobile: map becomes vertically scrollable (trail runs top-to-bottom); guide's note docks below; workspace goes full-screen with a compact "trail" progress pill in the header.

## Suggested stack

Any framework works. If React: single-page app, map as an inline SVG component with waypoint data driven from a config array (`{id, name, subtitle, x, y, state}`), trail path drawn through waypoint coordinates. Stage workspaces render behind a router or state machine keyed by waypoint id. Persist state so "make camp / break camp" resume works.
