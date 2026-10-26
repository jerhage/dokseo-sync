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

## Run it

Build it without cgo:

```sh
CGO_ENABLED=0 go build ./cmd/dokseo-sync
```

Each flag falls back to an environment variable, and a flag wins over the variable.

| Flag | Environment variable | Meaning |
| --- | --- | --- |
| `--listen` | `DOKSEO_SYNC_LISTEN` | Address to listen on. Default `127.0.0.1:8080`. |
| `--data` | `DOKSEO_SYNC_DATA` | Directory for `dokseo-sync.db`. Required. |
| `--cors-origin` | `DOKSEO_SYNC_CORS_ORIGINS` | Browser origin allowed to call the API, as `scheme://host[:port]`. Repeat the flag, or separate several with commas in the variable. |
| `--username` | `DOKSEO_SYNC_USERNAME` | Account username. Required. |
| `--password-file` | `DOKSEO_SYNC_PASSWORD_FILE` | File that holds the account password. |
| none | `DOKSEO_SYNC_PASSWORD` | The account password. Set this or the password file, not both. |

There is no password flag, because a flag shows in `ps`.

```sh
printf '%s\n' 'a long passphrase' > password.txt
chmod 600 password.txt
DOKSEO_SYNC_PASSWORD_FILE=password.txt ./dokseo-sync \
  --data ./data --username reader --cors-origin https://dokseo.example
```

The server listens on loopback by default and speaks plain HTTP. Put a reverse proxy or `tailscale serve` in front of it for HTTPS.

## License

MIT
