// The MIT License
//
// Copyright (c) 2020, Cloudflare, Inc. All rights reserved.
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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type testTarget struct {
	expectedStatusCode int
}

func (t testTarget) handleRequest(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	fmt.Println("here")

	headerContentType := r.Header.Get("Content-Type")
	w.Header().Set("Content-Type", headerContentType)
	w.Write(body)
}

func TestProxyMethod(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := queryEndpoint
	request, err := http.NewRequest("GET", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Failed when sent an invalid request method. Expected %d, got %d", http.StatusBadRequest, status))
	}
	if proxy.lastError != ErrWrongMethod {
		t.Fatal(fmt.Errorf("Incorrect error. Expected %s", ErrWrongMethod.Error()))
	}
}

func TestProxyQueryParametersMissing(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")

	testURLs := []string{
		queryEndpoint,
		"/not-the-right-endpoint",
		queryEndpoint + "?targethost=",
		queryEndpoint + "?targetpath=bar",
	}
	for _, url := range testURLs {
		request, err := http.NewRequest("POST", url, fakeQueryBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, request)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Fatal(fmt.Errorf("Failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
		}
		if proxy.lastError != ErrMissingTargetHost {
			t.Fatal(fmt.Errorf("Incorrect error. Expected %s", ErrMissingTargetHost.Error()))
		}
	}

	testURLs = []string{
		queryEndpoint + "?targethost=foo",
		queryEndpoint + "?targethost=foo&targetpath=",
	}
	for _, url := range testURLs {
		request, err := http.NewRequest("POST", url, fakeQueryBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, request)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Fatal(fmt.Errorf("Failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
		}
		if proxy.lastError != ErrMissingTargetPath {
			t.Fatal(fmt.Errorf("Incorrect error. Expected %s", ErrMissingTargetPath.Error()))
		}
	}
}

func TestProxyQueryMissingBody(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	emptyQueryBody := strings.NewReader("")
	request, err := http.NewRequest("POST", queryEndpoint+"?targethost=foo&targetpath=bar", emptyQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
	}
	if proxy.lastError != ErrEmptyRequestBody {
		t.Fatal(fmt.Errorf("Incorrect error. Expected %s", ErrEmptyRequestBody.Error()))
	}
}

func TestProxyIncorrectTarget(t *testing.T) {
	proxy := proxyServer{
		client: &http.Client{},
	}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := queryEndpoint + "?targethost=nottherighttarget.com&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Fatal(fmt.Errorf("Failed to propagate the desired error code. Expected %d, got %d", http.StatusInternalServerError, status))
	}
}

func TestProxyStatusCodePropagationOK(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	testTargetURL := testURL.Hostname() + ":" + testURL.Port()

	proxy := proxyServer{
		client: ts.Client(),
	}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := queryEndpoint + "?targethost=" + testTargetURL + "&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Fatal(fmt.Errorf("Failed to propagate the desired error code. Expected %d, got %d", http.StatusOK, status))
	}
}

func TestProxyStatusCodePropagationFailure(t *testing.T) {
	expectedFailure := http.StatusTeapot
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(expectedFailure), expectedFailure)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	testTargetURL := testURL.Hostname() + ":" + testURL.Port()

	proxy := proxyServer{
		client: ts.Client(),
	}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := queryEndpoint + "?targethost=" + testTargetURL + "&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != expectedFailure {
		t.Fatal(fmt.Errorf("Failed to propagate the desired error code. Expected %d, got %d", expectedFailure, status))
	}
}
