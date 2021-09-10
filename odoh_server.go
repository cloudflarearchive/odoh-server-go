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
	"fmt"
	"net/http"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// HTTP constants. Fill in your proxy and target here.
	defaultPort      = "443"
	proxyURI         = "https://odoh.proxy.shared.aka-mcqa.com"
	proxyEndpoint    = "/proxy"
	healthEndpoint   = "/health"
	defaultTokenFile = "./.AuthToken"

	// Environment variables
	logLocationEnvironmentVariable = "LOG"
	targetNameEnvironmentVariable  = "NAME"
	certificateEnvironmentVariable = "CERT"
	keyEnvironmentVariable         = "KEY"
)

type odohServer struct {
	endpoints map[string]string
	Verbose   bool
	proxy     *proxyServer
	DOHURI    string
}

func (s odohServer) indexHandler(w http.ResponseWriter, r *http.Request) {
	log.SetFormatter(&log.JSONFormatter{})
	log.WithFields(
		log.Fields{
			"Method": r.Method,
			"URL":    r.URL.Path,
		},
	).Info("Handling Index Request")
	// log.Printf("%s Handling %s\n", r.Method, r.URL.Path)
	fmt.Fprint(w, "ODOH Proxy service\n")
	fmt.Fprint(w, "----------------\n")
	fmt.Fprintf(w, "Proxy endpoint: https://%s%s{?targethost,targetpath}\n", r.Host, s.endpoints["Proxy"])
	fmt.Fprintf(w, "Proxy endpoint: https://%s%s\n", r.Host, s.endpoints["Health"])
	fmt.Fprint(w, "----------------\n")
}

func (s odohServer) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	log.SetFormatter(&log.JSONFormatter{})
	log.WithFields(
		log.Fields{
			"Method": r.Method,
			"URL":    r.URL.Path,
		},
	).Info("Handling Health Request")
	// log.Printf("%s Handling %s\n", r.Method, r.URL.Path)
	fmt.Fprint(w, "ok\n")
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	logLocation := os.Getenv("LOG")
	if logLocation == "" {
		logLocation = "./logs/"
	}
	err := os.MkdirAll(logLocation, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.OpenFile(logLocation+"/proxy.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(file)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	var serverName string
	if serverNameSetting := os.Getenv(targetNameEnvironmentVariable); serverNameSetting != "" {
		serverName = serverNameSetting
	} else {
		serverName = "server_localhost"
	}
	log.WithFields(
		log.Fields{
			"ServerName": serverName,
		},
	).Info("Starting Proxy Server")

	var certFile string
	if certFile = os.Getenv(certificateEnvironmentVariable); certFile == "" {
		certFile = "cert.pem"
	}

	var keyFile string
	enableTLSServe := true
	if keyFile = os.Getenv(keyEnvironmentVariable); keyFile == "" {
		keyFile = "key.pem"
		enableTLSServe = false
	}

	endpoints := make(map[string]string)
	endpoints["Proxy"] = proxyEndpoint
	endpoints["Health"] = healthEndpoint

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
		proxy:     proxy,
		DOHURI:    fmt.Sprintf("%s/%s", proxyURI, proxyEndpoint),
	}

	http.HandleFunc(proxyEndpoint, server.proxy.proxyQueryHandler)
	http.HandleFunc(healthEndpoint, server.healthCheckHandler)
	http.HandleFunc("/", server.indexHandler)

	if enableTLSServe {
		log.WithFields(
			log.Fields{
				"Port": port,
				"Cert": certFile,
				"Key":  keyFile,
			},
		).Info("Server Started With TLS Support")
		log.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%s", port), certFile, keyFile, nil))
	} else {
		log.WithFields(
			log.Fields{
				"Port": port,
			},
		).Info("Server Started Without TLS Support")
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
	}

}
