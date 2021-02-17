# odoh-server-go

[![Coverage Status](https://coveralls.io/repos/github/cloudflare/odoh-server-go/badge.svg?branch=master)](https://coveralls.io/github/cloudflare/odoh-server-go?branch=master)

[Oblivious DoH Server](https://tools.ietf.org/html/draft-pauly-dprive-oblivious-doh)

# Preconfigured Deployments

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy)
[![deploy to Scalingo](https://cdn.scalingo.com/deploy/button.svg)](https://my.scalingo.com/deploy)

# Local development

To deploy the server locally, first acquire a TLS certificate using [mkcert](https://github.com/FiloSottile/mkcert) as follows:

~~~
$ mkcert -key-file key.pem -cert-file cert.pem 127.0.0.1 localhost
~~~

Then build and run the server as follows:

~~~
$ make all
$ CERT=cert.pem KEY=key.pem PORT=4567 ./odoh-server
~~~

You may then run the [corresponding client](https://github.com/cloudflare/odoh-client-go) as follows:

~~~
$ ./odoh-client odoh --proxy localhost:4567 --target odoh.cloudflare-dns.com --domain cloudflare.com
;; opcode: QUERY, status: NOERROR, id: 14306
;; flags: qr rd ra; QUERY: 1, ANSWER: 2, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;cloudflare.com.	IN	 AAAA

;; ANSWER SECTION:
cloudflare.com.	271	IN	AAAA	2606:4700::6810:84e5
cloudflare.com.	271	IN	AAAA	2606:4700::6810:85e5
~~~

# Usage

To deploy, run:

~~~
$ gcloud app deploy proxy.yaml
...
$ gcloud app deploy target.yaml
...
~~~

To check on its status, run:

~~~
$ gcloud app browse
~~~

To stream logs when deployed, run

~~~
$ gcloud app logs tail -s default
~~~

To run locally build and run the project using

```shell
go build
PORT=8080 ./odoh-server-go
```

By default, the proxy listens on `/proxy` and the target listens on `/dns-query`.

## Reverse proxy

You need to deploy a reverse proxy with a valid TLS server certificate
for clients to be able to authenticate the target or proxy.

The simplest option for this is using [Caddy](https://caddyserver.com).
Caddy will automatically provision a TLS certificate using ACME from [Let's Encrypt](https://letsencrypt.org).

For instance:

```
caddy reverse-proxy --from https://odoh.example.net:443 --to 127.0.0.1:8080
```

Alternatively, use a Caddyfile similar to:

```
odoh.example.net

reverse_proxy localhost:8080
```
and run `caddy start`.
