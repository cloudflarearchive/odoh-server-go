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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type proxyServer struct {
	client    *http.Client
	lastError error
}

var (
	errWrongMethod       = fmt.Errorf("Unsupported method")
	errMissingTargetHost = fmt.Errorf("Missing proxy targethost query parameter")
	errMissingTargetPath = fmt.Errorf("Missing proxy targetpath query parameter")
	errEmptyRequestBody  = fmt.Errorf("Missing request body")
)

func forwardProxyRequest(client *http.Client, targetName string, targetPath string, body []byte, r *http.Request) (*http.Response, error) {
	targetURL := "https://" + targetName + targetPath
	req, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		log.WithFields(
			log.Fields{
				"Method": r.Method,
				"URL":    r.URL.Path,
				"Target": targetName,
				"Path":   targetPath,
			},
		).Info("Not Able to Create HTTP Request %s", err)
		// log.Println("Failed creating target POST request")
		return nil, errors.New("failed creating target " + r.Method + " request")
	}
	for name, values := range r.Header {
		// Loop over all values for the name.
		for _, value := range values {
			log.WithFields(
				log.Fields{
					"Method": r.Method,
					"URL":    r.URL.Path,
					"Target": targetName,
					"Path":   targetPath,
				},
			).Info("Setting Request Header " + name + ": " + value)
			req.Header.Set(name, value)
		}
	}

	return client.Do(req)
}

func (p *proxyServer) proxyQueryHandler(w http.ResponseWriter, r *http.Request) {
	log.SetFormatter(&log.JSONFormatter{})
	log.WithFields(
		log.Fields{
			"Method": r.Method,
			"URL":    r.URL.Path,
		},
	).Info("Handling Proxy Request")
	// log.Printf("%s Handling %s\n", r.Method, r.URL.Path)

	if r.Method != "POST" {
		p.lastError = errWrongMethod
		log.Printf(p.lastError.Error())
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	targetName := r.URL.Query().Get("targethost")
	if targetName == "" {
		p.lastError = errMissingTargetHost
		log.Printf(p.lastError.Error())
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	targetPath := r.URL.Query().Get("targetpath")
	if targetPath == "" {
		p.lastError = errMissingTargetPath
		log.Printf(p.lastError.Error())
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		p.lastError = errEmptyRequestBody
		log.Printf(p.lastError.Error())
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	log.WithFields(
		log.Fields{
			"Method": r.Method,
			"URL":    r.URL.Path,
			"Target": targetName,
			"Path":   targetPath,
		},
	).Info("Forwarding Request To Target")
	response, err := forwardProxyRequest(p.client, targetName, targetPath, body, r)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Method": r.Method,
				"URL":    r.URL.Path,
				"Target": targetName,
				"Path":   targetPath,
			},
		).Info("Error Response Received %s", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if response.StatusCode != 200 {
		log.WithFields(
			log.Fields{
				"Method": r.Method,
				"URL":    r.URL.Path,
				"Target": targetName,
				"Path":   targetPath,
			},
		).Info("Non OK Response Code Received %s", response.StatusCode)
		http.Error(w, http.StatusText(response.StatusCode), response.StatusCode)
		return
	}

	defer response.Body.Close()
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Method": r.Method,
				"URL":    r.URL.Path,
				"Target": targetName,
				"Path":   targetPath,
			},
		).Info("Not Able To Read Response Body %s", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for name, values := range response.Header {
		// Loop over all values for the name.
		for _, value := range values {
			log.WithFields(
				log.Fields{
					"Method": r.Method,
					"URL":    r.URL.Path,
					"Target": targetName,
					"Path":   targetPath,
				},
			).Info("Setting Response Header " + name + ": " + value)
			w.Header().Set(name, value)
		}
	}
	w.Write(responseBody)
}
