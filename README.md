# odohd

[Oblivious DoH Server](https://tools.ietf.org/html/draft-pauly-dprive-oblivious-doh) based on [Cloudflare's odoh-server-go](https://github.com/cloudflare/odoh-server-go)

![Coverage Badge](coverage_badge.png)
[![Go Report](https://goreportcard.com/badge/github.com/emeraldonion/odohd?style=for-the-badge)](https://goreportcard.com/report/github.com/emeraldonion/odohd)
[![License](https://img.shields.io/github/license/emeraldonion/odohd?style=for-the-badge)](https://raw.githubusercontent.com/emeraldonion/odohd/main/LICENSE)
[![Release](https://img.shields.io/github/v/release/emeraldonion/odohd?style=for-the-badge)](https://github.com/emeraldonion/odohd/releases)

This fork includes changes for a server suited to Emerald Onion's production deployment.

## Usage:

```
Usage:
  odohd [OPTIONS]

Application Options:
  -l, --listen=           Address to listen on (default: localhost:8080)
  -m, --metrics-listen=   Address to listen metrics server on (default: localhost:8081)
  -r, --resolver=         Target DNS resolver (default: 127.0.0.1:53)
  -t, --no-tls            Disable TLS
  -c, --cert=             TLS certificate file
  -k, --key=              TLS key file
      --resolver-timeout= Resolver timeout (seconds) (default: 2.5)
      --proxy-timeout=    Proxy timeout (seconds) (default: 2.5)
  -v, --verbose           Enable verbose logging
  -V, --version           Show version and exit

Help Options:
  -h, --help              Show this help message
```
