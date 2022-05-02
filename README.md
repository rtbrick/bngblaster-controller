# BNG Blaster Control Daemon

[![Build](https://github.com/rtbrick/bngblaster-controller/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/rtbrick/bngblaster-controller/actions/workflows/build.yml)
[![License](https://img.shields.io/badge/License-BSD-lightgrey)](https://github.com/rtbrick/bngblaster-controller/blob/main/LICENSE)
[![Documentation](https://img.shields.io/badge/Documentation-lightgrey)](https://rtbrick.github.io/bngblaster/controller.html)
[![API](https://img.shields.io/badge/API-green)](https://rtbrick.github.io/bngblaster-controller)

The [BNG Blaster](https://github.com/rtbrick/bngblaster) controller provides
a REST API to start and stop multiple test instances. It exposes the
BNG Blaster [JSON RPC API](https://rtbrick.github.io/bngblaster/api/index.html)
as REST API and provides endpoints to download logs and reports. 

## Usage

The controller comes with good defaults, just starting the controller will give you an instance that:

* runs on port `8001`
* assumes bngblaster is installed at `/usr/sbin/bngblaster`
* uses `/var/bngblaster` as storage directory 

The blaster instance needs at least the permissions required to run 
the `bngblaster` itself.

```
$ ./bngblasterctrl -h
Usage of bngblasterctrl:
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
    	bngblaster executable (default "/usr/sbin/bngblaster")
```

## License

BNG Blaster is licensed under the BSD 3-Clause License, which means that you are free to get and use it for
commercial and non-commercial purposes as long as you fulfill its conditions.

See the LICENSE file for more details.

## Copyright

Copyright (C) 2020-2022, RtBrick, Inc.

## Contact

bngblaster@rtbrick.com