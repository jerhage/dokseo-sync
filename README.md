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

## Container

Images are published to `ghcr.io/jerhage/dokseo-sync` for amd64 and arm64 on each version tag. A release `v0.1.2` gets the tags `0.1.2`, `0.1` and `latest`.

The image listens on `0.0.0.0:8080`, keeps the database in the `/data` volume and runs as uid 65532.

```sh
docker run -d --name dokseo-sync \
  -p 127.0.0.1:8080:8080 \
  -v dokseo-sync-data:/data \
  -v "$PWD/password.txt:/run/secrets/dokseo-sync-password:ro" \
  -e DOKSEO_SYNC_PASSWORD_FILE=/run/secrets/dokseo-sync-password \
  -e DOKSEO_SYNC_USERNAME=reader \
  -e DOKSEO_SYNC_CORS_ORIGINS=https://dokseo.example \
  ghcr.io/jerhage/dokseo-sync:latest
```

The password file must be readable by uid 65532, so give it mode `644` and keep it in a host directory that only you can enter. A Compose file secret is a bind mount too, so the same applies to it.

The same with Compose, using a file secret that mounts at `/run/secrets/<name>`:

```yaml
services:
  dokseo-sync:
    image: ghcr.io/jerhage/dokseo-sync:latest
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - data:/data
    environment:
      DOKSEO_SYNC_USERNAME: reader
      DOKSEO_SYNC_CORS_ORIGINS: https://dokseo.example
      DOKSEO_SYNC_PASSWORD_FILE: /run/secrets/dokseo-sync-password
    secrets:
      - dokseo-sync-password

volumes:
  data:

secrets:
  dokseo-sync-password:
    file: ./password.txt
```

The examples use `docker`; `podman` takes the same arguments. To build the image yourself:

```sh
podman build --build-arg VERSION=0.1.2 -t dokseo-sync .
```

Docker does not look for a `Containerfile` by itself, so name it:

```sh
docker build -f Containerfile --build-arg VERSION=0.1.2 -t dokseo-sync .
```

As with the binary, put a reverse proxy or `tailscale serve` in front of it for HTTPS.

## NixOS

This repo is a flake. Add it as an input and import the module:

```nix
{
  inputs.dokseo-sync.url = "github:jerhage/dokseo-sync";

  # in your nixosSystem modules list
  imports = [ inputs.dokseo-sync.nixosModules.default ];
}
```

Then, on the host:

```nix
services.dokseo-sync = {
  enable = true;
  username = "reader";
  passwordFile = "/run/secrets/dokseo-sync-password";
  corsOrigins = [ "https://dokseo.example" ];
};
```

The service runs as a `DynamicUser` and keeps its database in
`/var/lib/dokseo-sync`. `passwordFile` is read by root and handed over as a
systemd credential, so the service user never gets access to it; any secret
manager that can put a file on disk works.

`nix run github:jerhage/dokseo-sync -- --help` runs the binary without
installing anything.

## Backups

The database runs in WAL mode, so it is three files and copying it while the server runs can miss the newest changes. Take a consistent copy first and back that up:

```sh
sqlite3 /var/lib/dokseo-sync/dokseo-sync.db "VACUUM INTO '/tmp/dokseo-sync.db'"
```

In the container the database is `/data/dokseo-sync.db`. The image has no `sqlite3`, so run `VACUUM INTO` from the host against the volume's path, or from a sqlite3 container that mounts the same volume.

## License

MIT
