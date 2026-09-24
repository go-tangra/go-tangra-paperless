# Quickstart: Paperless Service

Validation scenarios proving the feature end to end. Prerequisites: the `deploy/stack`
compose brought up with the `paperless` service registered (`registered:true`), a
MinIO/RustFS object store, and (for extraction) Tika + Gotenberg reachable. Sign in to the
console; the **Paperless** menu appears once the remote is registered and the operator has
the paperless permissions.

## Scenario 1 — Upload and download (US1, MVP)

1. Open **Paperless → Documents → Upload**; choose a small file; save.
2. **Expected**: the document appears with its name, size, detected type, and a checksum;
   it is owned by you; status `active`, processing `pending`.
3. Download it → the bytes match the original. Request a download link → it retrieves the
   same bytes and expires after its TTL.

Verifies: FR-001/002/003, SC-001.

## Scenario 2 — Extraction and search (US2)

1. Upload a text-bearing document; watch its processing status move `pending → processing →
   completed` live (no refresh).
2. Search for a word that appears only in the document body.
3. **Expected**: the document is returned with a highlighted snippet; a document containing
   a different word is not; results are limited to documents you may read.

Verifies: FR-013/014/015/016, SC-002/SC-009.

## Scenario 3 — Categories (US3)

1. Create category `Finance`, then `2026` under it; upload a document into `2026`.
2. Fetch the tree → nesting, paths (`/Finance/2026`), and document counts are correct.
3. Move the document to `Finance` → its category path updates and both counts adjust.

Verifies: FR-006/007/008, SC (organization).

## Scenario 4 — Sharing with inheritance (US4)

1. As owner, grant another subject **viewer** on `Finance`.
2. As that subject, read a document inside `Finance/2026` → allowed by inheritance; attempt
   to edit → refused.
3. Revoke the grant → access is gone. Add an expiring grant → it lapses at expiry.

Verifies: FR-009/010/011/012, SC-003/SC-004.

## Scenario 5 — Statistics & backup (US5)

1. View statistics → document counts by status/type, storage usage, processing backlog.
2. Export the tenant → categories + document metadata + permissions (bytes referenced,
   not inlined). Import into an empty tenant → they are recreated.

Verifies: FR-017/018, SC (operability).

## Security checks (cross-cutting)

- No response, log, audit record, or backup contains object-store credentials or extracted
  content (SR-002, SC-005).
- A second tenant cannot read, search, download, or share the first tenant's documents
  (SR-003, SC-006).
- Two service instances never run the same extraction job twice (SC-007).
