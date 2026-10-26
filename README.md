# dokseo-sync

A small sync server for Dokseo, the manga reader that runs in the browser. It keeps an ordered log of record changes so the devices of one person end up with the same books, captures, tags, reading positions and catalogs. It does not merge anything and it never receives a book file.

## API

All bodies are JSON in UTF-8. Every route except the first two needs `Authorization: Bearer <token>`.

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| GET | `/v1/info` | none | Server name, version, supported API versions |
| POST | `/v1/devices` | username and password in the body | Register a device, receive its token once |
| GET | `/v1/devices` | bearer | List the account's devices |
| DELETE | `/v1/devices/{deviceId}` | bearer | Revoke a device's token |
| POST | `/v1/changes` | bearer | Push 1 to 500 changes, all or nothing |
| GET | `/v1/changes?after=&limit=` | bearer | Pull changes in order of `seq` |

The full contract, the JSON Schemas and examples are in [`contract/`](contract/). The rules a schema cannot express are in [`contract/openapi.yaml`](contract/openapi.yaml).

## License

MIT
