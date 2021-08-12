module github.com/cloudflare/odoh-server-go

// +heroku goVersion go1.14
// +scalingo goVersion go1.14
go 1.14

require (
	cloud.google.com/go/logging v1.1.1
	github.com/cisco/go-hpke v0.0.0-20210215210317-01c430f1f302
	github.com/cloudflare/odoh-go v1.0.0
	github.com/elastic/go-elasticsearch/v8 v8.0.0-20201022194115-1af099fb3eca
	github.com/miekg/dns v1.1.35
)
