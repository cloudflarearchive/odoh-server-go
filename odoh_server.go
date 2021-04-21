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
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cisco/go-hpke"
	"github.com/cloudflare/odoh-go"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

const (
	// HPKE constants
	kemID  = hpke.DHKEM_X25519
	kdfID  = hpke.KDF_HKDF_SHA256
	aeadID = hpke.AEAD_AESGCM128

	// Keying material (seed) should have as many bits of entropy as the bit length of the x25519 secret key
	defaultSeedLength = 32
)

// Set by build process
var version = "dev"

// CLI flags
var opts struct {
	ListenAddr string `short:"l" long:"listen" description:"Address to listen on" default:"localhost:8080"`
	Resolver   string `short:"r" long:"resolver" description:"Upstream DNS resolver to query" default:"1.1.1.1:53"`
	Cert       string `short:"c" long:"cert" description:"TLS certificate file" required:"true"`
	Key        string `short:"k" long:"key" description:"TLS key file" required:"true"`
	Verbose    bool   `short:"v" long:"verbose" description:"Enable verbose logging"`
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

	// Random seed for HPKE keypair
	seed := make([]byte, defaultSeedLength)
	_, err = rand.Read(seed)
	if err != nil {
		log.Fatal(err)
	}

	keyPair, err := odoh.CreateKeyPairFromSeed(hpke.DHKEM_X25519, hpke.KDF_HKDF_SHA256, hpke.AEAD_AESGCM128, seed)
	if err != nil {
		log.Fatal(err)
	}

	target := &targetServer{
		verbose: false,
		resolver: &targetResolver{
			timeout:    2500 * time.Millisecond,
			nameserver: opts.Resolver,
		},
		odohKeyPair: keyPair,
	}

	proxy := &proxyServer{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 0 * time.Second,
			},
		},
	}

	// HTTP handlers
	http.HandleFunc("/proxy", proxy.proxyQueryHandler)
	http.HandleFunc("/dns-query", target.targetQueryHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	})
	http.HandleFunc("/.well-known/odohconfigs", target.configHandler)

	// Start the server
	log.Infof("Starting ODoH listener on %s", opts.ListenAddr)
	log.Fatal(http.ListenAndServeTLS(opts.ListenAddr, opts.Cert, opts.Key, nil))
}
