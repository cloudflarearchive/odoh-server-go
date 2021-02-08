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
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cisco/go-hpke"
	odoh "github.com/cloudflare/odoh-go"
)

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

func (s odohServer) indexHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s Handling %s\n", r.Method, r.URL.Path)
	fmt.Fprint(w, "ODOH service\n")
	fmt.Fprint(w, "----------------\n")
	fmt.Fprintf(w, "Proxy endpoint: https://%s%s{?targethost,targetpath}\n", r.Host, s.endpoints["Proxy"])
	fmt.Fprintf(w, "Target endpoint: https://%s%s{?dns}\n", r.Host, s.endpoints["Target"])
	fmt.Fprint(w, "----------------\n")
}

func (s odohServer) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s Handling %s\n", r.Method, r.URL.Path)
	fmt.Fprint(w, "ok")
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

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

	var serverName string
	if serverNameSetting := os.Getenv(targetNameEnvironmentVariable); serverNameSetting != "" {
		serverName = serverNameSetting
	} else {
		serverName = "server_localhost"
	}
	log.Printf("Setting Server Name as %v", serverName)

	var experimentID string
	if experimentID := os.Getenv(experimentIDEnvironmentVariable); experimentID == "" {
		experimentID = "EXP_LOCAL"
	}

	var telemetryType string
	if telemetryType := os.Getenv(telemetryTypeEnvironmentVariable); telemetryType == "" {
		telemetryType = "LOG"
	}

	keyPair, err := odoh.CreateKeyPairFromSeed(kemID, kdfID, aeadID, seed)
	if err != nil {
		log.Fatal("Failed to create a private key. Exiting now.")
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
	http.HandleFunc(healthEndpoint, server.healthCheckHandler)
	http.HandleFunc(configEndpoint, target.configHandler)
	http.HandleFunc("/", server.indexHandler)

	log.Printf("Listening on port %v\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
