# BNG Blaster Control Daemon

[![Build](https://github.com/rtbrick/bngblaster-controller/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/rtbrick/bngblaster-controller/actions/workflows/build.yml)
[![License](https://img.shields.io/badge/License-BSD-lightgrey)](https://github.com/rtbrick/bngblaster-controller/blob/main/LICENSE)
[![Documentation](https://img.shields.io/badge/Documentation-lightgrey)](https://rtbrick.github.io/bngblaster/controller.html)
[![API](https://img.shields.io/badge/API-green)](https://rtbrick.github.io/bngblaster-controller)

The [BNG Blaster](https://github.com/rtbrick/bngblaster) controller provides
a REST API to start and stop multiple test instances. It exposes the
BNG Blaster [JSON RPC API](https://rtbrick.github.io/bngblaster/api/index.html)
as REST API and provides endpoints to download logs and reports. 

![BNG Blaster Controller](docs/controller.png "BNG Blaster Controller")

## Installation

Pre-built debian packages, as well as plain `tar.gz` archives, are
published on the [GitHub releases page](https://github.com/rtbrick/bngblaster-controller/releases).

Installing the Debian package registers and starts a systemd service:

```
$ sudo dpkg -i bngblaster-controller_<version>_amd64.deb
```

This installs the `bngblasterctrl` binary to `/usr/local/bin/bngblasterctrl`,
a systemd unit (`rtbrick-bngblasterctrl.service`), a default environment
file at `/etc/default/rtbrick-bngblasterctrl` (see
[Configuration](#configuration) below), and a logrotate policy at
`/etc/logrotate.d/rtbrick-bngblasterctrl` for the service's stdout/stderr log
files under `/var/log/`. The service is enabled and started automatically.

Alternatively, build from source:

```
$ make build
$ sudo ./bin/<os>_<arch>/bngblasterctrl
```

The blaster instance needs at least the permissions required to run
the `bngblaster` itself.

## Usage

The controller comes with good defaults, just starting the controller will give you an instance that:

* runs on port `8001`
* assumes bngblaster is installed at `/usr/bin/bngblaster`
* uses `/var/bngblaster` as storage directory 

```
$ /usr/local/bin/bngblasterctrl -h
Usage of /usr/local/bin/bngblasterctrl:
  -addr string
    	HTTP network address (default ":8001")
  -color
    	turn on color of color output
  -console
    	turn on pretty console logging (default true)
  -d string
    	config folder (default "/var/bngblaster")
  -debug
    	turn on debug logging
  -e string
    	bngblaster executable (default "/usr/bin/bngblaster")
  -interfaces-api
    	enable the interfaces endpoint
  -schema string
    	path to the bngblaster configuration JSON schema served on /api/v1/schema (default "/etc/bngblaster/bngblaster-config.json")
  -ui
    	enable the embedded web UI (experimental)
  -upload
    	enable file upload
```

## Configuration

### Command line

All options above can be passed directly on the command line when running
`bngblasterctrl` manually.

### systemd service

When installed via the debian package, the service is started by
systemd and does not take command-line arguments directly. Instead, flags are
configured through `/etc/default/rtbrick-bngblasterctrl`, which is sourced by
the unit as an `EnvironmentFile` and expanded into `ExecStart` via the
`BNGBLASTERCTRL_OPTS` variable:

```
# /etc/default/rtbrick-bngblasterctrl
BNGBLASTERCTRL_OPTS="-addr :8080 -d /var/bngblaster"
```

After editing the file, apply the change with:

```
$ sudo systemctl restart rtbrick-bngblasterctrl
```

This file is preserved across package upgrades and is the recommended way to
configure the service; editing the unit file directly (e.g. via
`systemctl edit rtbrick-bngblasterctrl`) also works but is not required.

## Experimental Web UI

The controller ships with an embedded, experimental web UI for creating and
observing test instances without calling the REST API directly. It is
**disabled by default**, along with the two additional endpoints it depends
on:

* `-ui` — serves the web UI on `/`
* `-interfaces-api` — serves `/api/v1/interfaces`, used by the web UI to
  populate the host network interface dropdown when creating a new instance
* `-upload` — enables the `/api/v1/instances/{instance_name}/_upload`
  endpoint, used by the web UI (and the REST API) to upload files into a
  test instance

To try it out, start (or configure the systemd service to start) the
controller with all three flags enabled:

```
$ /usr/local/bin/bngblasterctrl -ui -interfaces-api -upload
```

or, for the systemd-installed service, in `/etc/default/rtbrick-bngblasterctrl`:

```
BNGBLASTERCTRL_OPTS="-ui -interfaces-api -upload"
```

Then open `http://<host>:<port>/` in a browser. As the UI is experimental,
expect rough edges, and only enable it on networks you trust, since none of
these endpoints require authentication yet.

## License

BNG Blaster is licensed under the BSD 3-Clause License, which means that you are free to get and use it for
commercial and non-commercial purposes as long as you fulfill its conditions.

See the LICENSE file for more details.

## Copyright

Copyright (C) 2020-2026, RtBrick, Inc.

## Contact

bngblaster@rtbrick.com