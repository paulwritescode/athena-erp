# Summary: F4 slices 2+3 train (RecordsPage embeds + QueueMessages + lists Desk UI)

Merged 2026-11-25 as 11 commits (tip 9089f8d).

Scope. Slice 3 (bottom 9): lists:send permission (gate + Roles UI) +
QueueMessages with audience rule + dry_run preview + lists Desk UI on the
Reports page (list/open/filter-text edit + gallery, Export PDF button, audience
preview confirm dialog) + Reports tests + docs + migration 23/24 tests +
main-rebase webfix. Slice 2 (top 2): Advanced filter (F4 builder behind the F2
seam) + saved-list chips on RecordsPage.vue (F4s2 owned RecordsPage until
merge per Stylus GO) + design note (FieldSettingsDialog embed owned by F3s2).
Stacked inverted (slice 3 below slice 2) with all evidence on the stacked
tree, so merged as one train rather than restacked.

Gates: worker Go affected + web 728/728 + typecheck + build + browser green on
the branch; one NEEDS_CHANGES cycle (branch carried inherited F4s1 conflict
markers + stale base — suite refused to gate; rebased, markers gone, behavior
unchanged, re-gated web 735/735 + both browsers green).
Review: r-f4 PASS (delta on rebased tip 5a34cb5: markers gone, exact-union
principal.go/gate.go/main.go appends, footprint identical; FULL make test green
22 Go pkgs + 735 web + fresh queue shot eye-checked).
Merge gate: full `make test` exit 0 clean, 693s, 22/22 Go pkgs ok + 735 web
(log 1125-032326-master.log). Picks clean onto post-C5s8 main.

Unblocks on merge: C5 slice 7 + C7 slice 9b RecordsPage touches, F5 sample
(dispatch w-f2). Field-to-field compare still open (allergen clash pending).

Branch `feat/f4-design` done (all 3 slices merged); tree kept until w-fu stands
down.
