# Contract

`openapi.yaml` describes the routes. `schemas/` holds one JSON Schema (draft 2020-12) per body and per record kind. `examples/valid/<schema>/` and `examples/invalid/<schema>/` hold bodies for each schema. Each invalid example breaks exactly one rule, and the file name states which.

A JSON Schema checks shape only. This file lists the rules it cannot express. Where the two disagree, a client should fix itself to match the schema and this file.

## What the server does

The server stores an ordered log of changes. It does not merge, interpret or reorder records. Every change gets a `seq` that is strictly increasing and starts at 1. A device pulls the log from the last `seq` it has seen and applies each change itself. Book files never go through the server.

## Ordering

- `seq` is the order in which the server stored changes. It is the order of delivery, not an order of precedence between edits.
- `GET /v1/changes` returns changes with `seq` greater than `after`, ascending. It includes the caller's own changes.
- `next` is the `seq` of the last change returned, or `after` when none came back. Pass it as the next `after`.
- `more` is true when changes with a higher `seq` exist at the time of the response.
- Within one push, new changes get increasing seqs in request order. `accepted` has one entry per change, in request order.

## Idempotency

- A change `id` that is already stored returns its existing `seq` and is not stored again. The stored content is not replaced, even if the new body differs.
- If one request holds the same `id` twice, both entries get the same `seq`.
- A client may therefore retry a push after a timeout without checking whether it landed.

## All or nothing

A push either stores every change or none. If any change is invalid the response is 400 `invalid-change` and `index` is the position in `changes` of the first invalid change. Nothing is stored, including changes before that index.

## Limits

| Limit | Value | Failure |
| --- | --- | --- |
| Request body | 1 MiB | 413 `too-large` |
| Changes per push | 1 to 500 | 400 `invalid-request` for 0 or over 500 |
| Serialized `data` of one change | 64 KiB | 400 `invalid-change` with `index` |
| `updatedAt` | greater than 0, at most 24 hours ahead of server time | 400 `invalid-change` with `index` |
| `limit` on pull | 1 to 1000, default 500 | 400 `invalid-request` |
| `after` on pull | integer 0 or more, default 0 | 400 `invalid-request` |

The 64 KiB limit counts the UTF-8 bytes of `data` serialized as JSON. No example holds 64 KiB, so there is no `change-data-too-large` example, and none holds 501 changes. Both rules are described here only.

## Rules a schema cannot express

- `recordId` for `book` and `position` is the book's sync key, the same 32 lowercase hex characters as `contentHash` in the book's data. For `capture`, `tag` and `catalog` it is a lowercase uuid. The `change` schema checks these patterns per `kind`.
- `deviceId` must equal the device of the bearer token, otherwise 403 `device-mismatch`. That error has no `index`.
- `data: null` is a tombstone: the record was deleted. A tombstone is an ordinary change and follows the merge rule below.
- The `change` envelope may carry extra fields. The server ignores and does not store them. Fields inside `data` are kept as sent, so a newer client can add fields and older clients ignore them.
- A capture's `bookKey` equals the `recordId` of its book. A capture's `tagIds` hold `recordId` values of tags. A book's `origin.catalogId` holds the `recordId` of a catalog. The server checks none of these links, and a change may arrive before the record it points to.
- Image rectangles are fractions of the page: `x + width` and `y + height` must be at most 1. A position's `shownThrough` must be at least its `index`. A capture's `editedAt` must not be before its `createdAt`. The server does not check these.
- A catalog record never holds a password or any other credential. `catalog-data` rejects a `password` field in the record and in `auth`. The server must not store a record that has one.
- `lastSeenAt` of a device is null until its first authenticated request.
- Timestamps are Unix milliseconds. `createdAt`, `editedAt`, `lastReadAt` and similar fields inside `data` are mirrored from Dokseo as plain numbers. `updatedAt`, `receivedAt` and the device timestamps are integers.

## Merge rule for clients

The server does not merge. Each client applies a pulled change to its own copy of a record, identified by `kind` and `recordId`, with this rule:

1. The change with the greater `updatedAt` wins.
2. When `updatedAt` is equal, the greater `deviceId` wins, compared as lowercase text.
3. The loser is dropped. This applies to a tombstone and to a live record alike.

Applying the same change twice, or applying changes in a different order, gives the same result. A client should treat its own changes coming back in a pull as ordinary changes.

## Auth

- `GET /v1/info` and `POST /v1/devices` need no bearer token. Everything else needs `Authorization: Bearer <token>`.
- A missing, unknown or revoked token is 401 `unauthorized`.
- `POST /v1/devices` checks `username` and `password` against the account and returns `deviceId` and `token`. The token is shown once. The server stores only its SHA-256.
- Wrong credentials are 401 `unauthorized`.
- `DELETE /v1/devices/{deviceId}` revokes that device's token and answers 204. A device may revoke itself. An unknown id is 404 `not-found`.

## Errors

Every error body is `{ "error": { "code", "message", "index"? } }`.

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid-request` | Body is not JSON, a field is missing or out of range, a query value is invalid |
| 400 | `invalid-change` | A change breaks a rule. Carries `index` |
| 401 | `unauthorized` | Missing, unknown or revoked token, or wrong credentials |
| 403 | `forbidden` | Authenticated but not allowed |
| 403 | `device-mismatch` | A change names a device other than the token's |
| 404 | `not-found` | No such device |
| 404 | `unsupported-version` | Path under an unknown `/vN/` |
| 413 | `too-large` | Body over 1 MiB |
| 429 | `rate-limited` | Too many requests |
| 500 | `internal` | Unexpected failure |

When several rules fail at once the order is: 413, 401, 400 `invalid-request`, 403 `device-mismatch`, 400 `invalid-change`.

## CORS

Dokseo runs in a browser, so the server answers preflight itself. It does this for configured origins only.

- `Access-Control-Allow-Origin` echoes the request origin when it is configured, and is absent otherwise.
- `Access-Control-Allow-Headers: Authorization, Content-Type`
- `Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS`
- No `Access-Control-Allow-Credentials`. Requests carry a bearer token and no cookies, so clients use `credentials: 'omit'`.
- `Vary: Origin` on every response that depends on the origin.

## Source of each record shape

Every `data` schema mirrors a type in Dokseo (github.com/jerhage/dokseo). Field names and value sets are the stored ones. A field is required when the stored row always has it.

| Schema | Source files and types |
| --- | --- |
| `book-data` | `src/lib/domains/storage/use-cases/captures-file.ts` (`FileBook`); `src/lib/domains/library/domain/book/stored-book.ts` (`bookFromStored`); `src/lib/domains/library/domain/book/book.ts` (`SourceKind`); `src/lib/shared/language.ts` (`Language`); `src/lib/shared/layout-kind.ts` (`LayoutKind`, `ReadingDirection`); `src/lib/shared/ids.ts` (partial MD5 form of `ContentHash`) |
| `book-data` `origin` | `src/lib/domains/catalog/domain/book-origin.ts` (`BookOrigin`, without `bookId`); `src/lib/domains/catalog/domain/stored-origin.ts` (`originFromStored`); `src/lib/domains/catalog/domain/remote-publication.ts` (`Acquisition`, `AcquisitionFormat`, `FeedPath`) |
| `capture-data` | `src/lib/domains/storage/use-cases/captures-file.ts` (`FileCapture`, `CapturesFile`); `src/lib/domains/storage/use-cases/export-captures.ts`; `src/lib/domains/recognition/domain/capture/capture.ts` (`Capture`, `captureFromStored`); `src/lib/shared/anchor.ts` (`Anchor`, `TextAnchor`, `TextQuote`); `src/lib/shared/image-region.ts` (`ImageRegion`); `src/lib/shared/geometry.ts` (`PageRect`, `fitsOnPage`); `src/lib/shared/capture-origin.ts` (`CaptureOrigin`) |
| `tag-data` | `src/lib/domains/recognition/domain/tag/tag.ts` (`Tag`, `tagFromStored`); `src/lib/domains/recognition/domain/tag/tag-colour.ts` (`TAG_COLOURS`) |
| `position-data` | `src/lib/shared/reading-place.ts` (`ReadingPlace`, `ImagePlace`, `TextPlace`); `src/lib/domains/library/domain/book/stored-book.ts` (`storedPlace`, `lastReadAt`, `finishedAt`); `src/lib/domains/viewing/domain/reading-position.ts` (`ReadingPosition`) |
| `catalog-data` | `src/lib/domains/catalog/domain/catalog.ts` (`Catalog`, `CatalogAuth`); `src/lib/domains/catalog/domain/stored-catalog.ts` (`catalogFromStored`, `storedCatalogRow`); `src/lib/domains/catalog/domain/catalog-protocol.ts` (`CatalogProtocol`) |

Differences from the stored types, each deliberate:

- A capture's `id` is the change's `recordId`, so `capture-data` has no `id`. A capture names its book with `bookKey` instead of the device-local `bookId`. This is the same replacement the export file makes.
- A book's `id`, `addedAt`, `pagePairing` and `pageFit` stay on the device and are not in `book-data`. Its `position`, `lastReadAt` and `finishedAt` travel in the `position` kind. `seriesId` and `volume` are optional in `book-data`.
- `position-data` follows `ReadingPlace`, which is what a book stores, and not the bare `ReadingPosition`: an image position has `index`, `shownThrough` and `offset`, and a text position has `cfi` and `fraction`. It also carries `lastReadAt` and `finishedAt` as optional fields.
- A catalog's `auth` holds only `kind` and, for `basic`, the `username`. Dokseo keeps the password for the session only, and it never leaves the device.

## Checking examples

Each example validates against its schema with a draft 2020-12 validator, with `$ref` values resolved relative to `schemas/`. Valid examples must pass and invalid examples must fail. Schema ids are the file names, such as `change.schema.json`.
