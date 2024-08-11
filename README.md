# filecoin-spade-client
[![Made by](https://img.shields.io/badge/made%20by-DukeSoft-blue.svg?style=flat-square)](https://dukesoft.nl)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](http://opensource.org/licenses/MIT)

> An application to automatically fetch and seal deals through [Spade](https://github.com/ribasushi/spade), using [Lotus](https://github.com/filecoin-project/lotus) and [Boost](https://github.com/filecoin-project/boost)

## Table of Contents

- [Install](#install)
- [Configure](#configure)
- [Contribute](#contribute)
- [License](#license)

## Install

Either download the binary or build this repository from source. 
`go run .` or `go build` in the `cmd/spade-client` folder should suffice.

To build properly linked `CGO_ENABLED=0 go build .`.

## Configure

Lotus and Boost configuration comes from the following environment variables;
```shell
FULLNODE_API_INFO
MINER_API_INFO
MARKETS_API_INFO
```

Please be sure these are properly set.

## Example
```shell
spade-client run --download-path /tmp/downloadfolder/ --max-spade-deals-active 2
```

## Usage
```text
NAME:
   spade-client - Filecoin Spade Client

USAGE:
   spade-client [global options] command [command options] [arguments...]

VERSION:
   1.1.0

DESCRIPTION:
   A client for Filecoin's Spade service

COMMANDS:
   run, r   Runs DukeSoft's Spade Client for Lotus
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  show version information (default: false)
```

### Run usage
```text
NAME:
   spade-client run - Runs DukeSoft's Spade Client for Lotus

USAGE:
   spade-client run [command options] [arguments...]

OPTIONS:
   --download-path value           The location where the downloaded files should reside (default: "/tmp/filecoin-spade-downloads")
   --max-spade-deals-active value  Total number of spade deals that should be actively downloading / requesting (This doesn't include other deals or sealing!) (default: 2)
   --boost-graphql-port value      Boost's GraphQL port (default: 8080)
   --help, -h                      show help
```

## Contribute

Contributions welcome. Please check out [the issues](https://github.com/dukesoft/filecoin-spade-client/issues).

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

[MIT](LICENSE) © 2024 DukeSoft BV
