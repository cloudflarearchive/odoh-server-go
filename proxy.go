// The MIT License
//
// Copyright (c) 2019 Apple, Inc.
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
	"io/ioutil"
	"log"
	"net/http"
)

type proxyServer struct {
	client *http.Client
}

func forwardProxyRequest(client *http.Client, targetName string, targetPath string, body []byte, headerContentType string) ([]byte, error) {
	req, err := http.NewRequest("POST", "https://"+targetName+targetPath, bytes.NewReader(body))
	if err != nil {
		log.Println("Failed creating target POST request")
		return nil, errors.New("failed creating target POST request")
	}
	req.Header.Set("Content-Type", headerContentType)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send proxied message %v\n", err)
		return nil, errors.New("failed to send proxied message")
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	return responseBody, err
}

func (p *proxyServer) proxyQueryHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s Handling %s\n", r.Method, r.URL.Path)

	if r.Method != "POST" {
		log.Printf("Unsupported method for %s", r.URL.Path)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	targetName := r.URL.Query().Get("targethost")
	if targetName == "" {
		log.Println("Missing proxy targethost query parameter in POST request")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	targetPath := r.URL.Query().Get("targetpath")
	if targetPath == "" {
		log.Println("Missing proxy targetpath query parameter in POST request")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Missing proxy message body in POST request")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	headerContentType := r.Header.Get("Content-Type")

	responseBody, err := forwardProxyRequest(p.client, targetName, targetPath, body, headerContentType)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", headerContentType)
	w.Write(responseBody)
}
