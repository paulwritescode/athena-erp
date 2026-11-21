Review B9 server (form layout and Image field), read-only, in /Users/iampaulking/projects/duchess/athena-erp-b9-server. Brief: .agents/tasks/beta-b9/server-brief.md; report: .agents/tasks/beta-b9/server-report.md.
Check:
1. Layout validation (unknown or duplicate keys, unplaced fields appended, CEL type-check, limits), round trip, layout-only save without DDL through the journaled publisher, revision and History.
2. Image uploads are a security surface: the type comes from content sniffing, not the name; the size limit is enforced before reading everything into memory; EXIF is stripped; the thumbnail decoder is bounded (no decompression bomb: check pixel dimensions before decoding fully); files are served same-origin with a safe Content-Type, nosniff and no inline SVG; alt text is required; path traversal is impossible.
3. RecordService accepts and returns the image reference, only through existing Grants; clearing works; deleting a Record keeps file versions.
4. Proto additive only; migration number and ADR-0027 if used; procedurePolicies; make generate stable.
5. Run gofmt -l, go build, go vet, make test.
Merge-small rule: BLOCKERs only stop the merge. Reply: hc send t6 "B9 SERVER REVIEW: PASS or FAIL" with file:line. Do not edit, commit or start agents.
