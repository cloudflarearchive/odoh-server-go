// The MIT License
//
// Copyright (c) 2019-2020, Cloudflare, Inc. and Apple, Inc. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cisco/go-hpke"
	"github.com/cloudflare/odoh-go"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

// Set by build process
var version = "dev"

// CLI flags
var opts struct {
	ListenAddr string `short:"l" long:"listen" description:"Address to listen on" default:"localhost:8080"`
	Cert       string `short:"c" long:"cert" description:"TLS certificate file"`
	Key        string `short:"k" long:"key" description:"TLS key file"`
	Verbose    bool   `short:"v" long:"verbose" description:"Enable verbose logging"`
}

const (
	// HPKE constants
	kemID  = hpke.DHKEM_X25519
	kdfID  = hpke.KDF_HKDF_SHA256
	aeadID = hpke.AEAD_AESGCM128

	// keying material (seed) should have as many bits of entropy as the bit
	// length of the x25519 secret key
	defaultSeedLength = 32

	// HTTP constants. Fill in your proxy and target here.
	defaultPort    = "8080"
	proxyURI       = "https://dnstarget.example.net"
	targetURI      = "https://dnsproxy.example.net"
	proxyEndpoint  = "/proxy"
	queryEndpoint  = "/dns-query"
	healthEndpoint = "/health"
	configEndpoint = "/.well-known/odohconfigs"

	// Environment variables
	secretSeedEnvironmentVariable    = "SEED_SECRET_KEY"
	targetNameEnvironmentVariable    = "TARGET_INSTANCE_NAME"
	experimentIDEnvironmentVariable  = "EXPERIMENT_ID"
	telemetryTypeEnvironmentVariable = "TELEMETRY_TYPE"
)

var (
	// DNS constants. Fill in a DNS server to forward to here.
	nameServers = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
)

type odohServer struct {
	endpoints map[string]string
	Verbose   bool
	target    *targetServer
	proxy     *proxyServer
	DOHURI    string
}

func main() {
	// Parse cli flags
	_, err := flags.ParseArgs(&opts, os.Args)
	if err != nil {
		os.Exit(1)
	}

	// Enable debug logging in development releases
	if //noinspection GoBoolExpressions
	version == "devel" || opts.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	log.Infof("Starting bcg %s", version)

	var seed []byte
	if seedHex := os.Getenv(secretSeedEnvironmentVariable); seedHex != "" {
		log.Printf("Using Secret Key Seed : [%v]", seedHex)
		var err error
		seed, err = hex.DecodeString(seedHex)
		if err != nil {
			panic(err)
		}
	} else {
		seed = make([]byte, defaultSeedLength)
		rand.Read(seed)
	}

	keyPair, err := odoh.CreateKeyPairFromSeed(kemID, kdfID, aeadID, seed)
	if err != nil {
		log.Fatal(err)
	}

	endpoints := make(map[string]string)
	endpoints["Target"] = queryEndpoint
	endpoints["Proxy"] = proxyEndpoint
	endpoints["Health"] = healthEndpoint
	endpoints["Config"] = configEndpoint

	resolversInUse := make([]resolver, len(nameServers))

	for index := 0; index < len(nameServers); index++ {
		resolver := &targetResolver{
			timeout:    2500 * time.Millisecond,
			nameserver: nameServers[index],
		}
		resolversInUse[index] = resolver
	}

	target := &targetServer{
		verbose:            false,
		resolver:           resolversInUse,
		odohKeyPair:        keyPair,
		telemetryClient:    getTelemetryInstance(telemetryType),
		serverInstanceName: serverName,
		experimentId:       experimentID,
	}

	proxy := &proxyServer{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 0 * time.Second,
			},
		},
	}

	server := odohServer{
		endpoints: endpoints,
		target:    target,
		proxy:     proxy,
		DOHURI:    fmt.Sprintf("%s/%s", targetURI, queryEndpoint),
	}

	http.HandleFunc(proxyEndpoint, server.proxy.proxyQueryHandler)
	http.HandleFunc(queryEndpoint, server.target.targetQueryHandler)
	http.HandleFunc(healthEndpoint, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	http.HandleFunc(configEndpoint, target.configHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ODOH service\n")
		fmt.Fprint(w, "----------------\n")
		fmt.Fprintf(w, "Proxy endpoint: https://%s%s{?targethost,targetpath}\n", r.Host, server.endpoints["Proxy"])
		fmt.Fprintf(w, "Target endpoint: https://%s%s{?dns}\n", r.Host, server.endpoints["Target"])
		fmt.Fprint(w, "----------------\n")
	})

	log.Infoln("Starting ODoH listener on %s", opts.ListenAddr)
	log.Fatal(http.ListenAndServeTLS(opts.ListenAddr, opts.Cert, opts.Key, nil))
}
