# odoh-server-go

[![Coverage Status](https://coveralls.io/repos/github/cloudflare/odoh-server-go/badge.svg?branch=master)](https://coveralls.io/github/cloudflare/odoh-server-go?branch=master)

[Oblivious DoH Server](https://tools.ietf.org/html/draft-pauly-dprive-oblivious-doh)

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

By default, the proxy listens on `/proxy` and the target listens on `/dns-query`.

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

# Deployment

This section describes deployment instructions for odoh-server-go.

## Preconfigured deployments

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy)
[![deploy to Scalingo](https://cdn.scalingo.com/deploy/button.svg)](https://my.scalingo.com/deploy)

## Manual deployment

This server can also be manually deployed on any bare metal machine, or in cloud providers such
as GCP. Instructions for both follow.

### Bare metal

Deployment on bare metal servers, such as [Equinix](https://metal.equinix.com/), can be done following
the instructions below. These steps assume that `git` and `go` are both installed on the metal.

1. Configure a certificate on the metal using [certbot](https://certbot.eff.org/all-instructions).
Once complete, the output should be something like the following, assuming the server domain name
is "example.com":

```
Successfully received certificate.
Certificate is saved at: /etc/letsencrypt/live/example.com/fullchain.pem
Key is saved at:         /etc/letsencrypt/live/example.com/privkey.pem
```

You must configure certbot to renew this certificate periodically. The simplest way to do this is
via a cron job:

```
$ 00 00 1 * 1 certbot renew
```

2. Configure two environment variables to reference these files:

```
$ export CERT=/etc/letsencrypt/live/example.com/fullchain.pem
$ export KEY=/etc/letsencrypt/live/example.com/privkey.pem
```

3. Clone and build the server:

```
$ git clone git@github.com:cloudflare/odoh-server-go.git
$ cd odoh-server-go
$ go build ./...
```

4. Run the server:

```
$ PORT=443 ./odoh-server &
```

This will run the server until completion. You must configure the server to restart should it
terminate prematurely.

### GCP

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

### Reverse proxy

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
