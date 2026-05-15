# torrents

A self-hostable torrent client and manager. The server manages a set of torrents added by magnet URI, downloads their content to a configurable directory, and exposes everything over an HTTP API, a web UI, and a CLI. State (added torrents, piece completion, labels) is persisted in SQLite so downloads survive restarts.

## Features

- **HTTP API** for adding, listing, pausing, resuming and removing torrents
- **Web UI** with live progress updates for browser-based management
- **CLI** for terminal and scripting workflows
- **Go client library** for programmatic access
- Single Go binary - no external services required
- SQLite-backed state, so torrent metadata and piece completion survive restarts

## Installation

The server is available as a binary for Windows, Mac & Linux as well as a Docker image. Binaries can be obtained from the [releases page](https://github.com/dsb-labs/torrents/releases) while Docker images can be pulled from [ghcr.io](https://github.com/dsb-labs/torrents/pkgs/container/torrents).

### Docker

```sh
docker run -d \
  -p 7373:7373 \
  -v $(pwd)/data:/data \
  ghcr.io/dsb-labs/torrents
```

Without a config file the server uses defaults. The web UI will be available at `http://localhost:7373`.

To customise settings, mount a config file and pass its path to `serve`:

```sh
docker run -d \
  -p 7373:7373 \
  -v $(pwd)/config.toml:/etc/torrents/config.toml \
  -v $(pwd)/data:/data \
  ghcr.io/dsb-labs/torrents serve /etc/torrents/config.toml
```

### Binary

Download the latest release for your platform, then run:

```sh
torrents serve config.toml
```

## Configuration

The server is operated via a single `serve` command, which accepts an optional configuration file. Run `torrents --help` for detailed usage information.

By default the server binds the HTTP API to `:7373`, stores its state under `$HOME/.local/share/torrents`, and logs at `info` level. To override any of these, pass a TOML configuration file:

```toml
[http]
# Address and port the HTTP server binds to (host:port).
address = "0.0.0.0:7373"

[data]
# Directory under which the SQLite database, torrent metainfo, and downloaded
# content are stored. The server creates `state.db` and a `downloads/` directory
# here.
directory = "/var/lib/torrents"

[logging]
# Log verbosity (debug, info, warn, error).
level = "info"
```

> [!NOTE]
> The HTTP `address` only controls the management API and web UI. BitTorrent peer and DHT ports are chosen by the underlying client and are not currently configurable.

Then pass the file into the `serve` command:

```sh
torrents serve path/to/config.toml
```

## HTTP API

All endpoints return JSON. Torrents are identified by their info hash.

| Method   | Path                              | Description                          |
|----------|-----------------------------------|--------------------------------------|
| `POST`   | `/api/v1/torrents`                | Add a torrent by magnet URI          |
| `GET`    | `/api/v1/torrents`                | List managed torrents                |
| `GET`    | `/api/v1/torrents/{hash}`         | Get a single managed torrent         |
| `DELETE` | `/api/v1/torrents/{hash}`         | Remove a managed torrent             |
| `POST`   | `/api/v1/torrents/{hash}/pause`   | Pause a managed torrent              |
| `POST`   | `/api/v1/torrents/{hash}/resume`  | Resume a managed torrent             |

## CLI

The `torrents` binary doubles as a CLI for talking to a running server. All client commands accept `--address` (default: `http://localhost:7373`).

| Command          | Description                                       |
|------------------|---------------------------------------------------|
| `serve`          | Start the server                                  |
| `add <magnet>`   | Add a torrent by magnet URI                       |
| `list` (`ls`)    | List managed torrents                             |
| `delete` (`rm`)  | Remove a managed torrent by info hash             |
| `pause`          | Pause a managed torrent by info hash              |
| `resume`         | Resume a managed torrent by info hash             |

## Go Client Library

A Go client library is available at `github.com/dsb-labs/torrents/pkg/client` for talking to the server programmatically. See the [package documentation](https://pkg.go.dev/github.com/dsb-labs/torrents/pkg/client) for the full API reference.

## Building from Source

**Requirements:** Go 1.26+, Node.js, Yarn

```sh
git clone https://github.com/dsb-labs/torrents
cd torrents

# Install Node dependencies for the web UI
yarn --cwd internal/server/ui install --frozen-lockfile

# Run code generation (mocks, templ, license bundle)
go generate ./...

# Build the binary
go build -o torrents .
```
