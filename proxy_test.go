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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProxyMethod(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := "/dns-query"
	request, err := http.NewRequest("GET", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("failed when sent an invalid request method. Expected %d, got %d", http.StatusBadRequest, status))
	}
	if proxy.lastError != errWrongMethod {
		t.Fatal(fmt.Errorf("incorrect error, expected %s", errWrongMethod.Error()))
	}
}

func TestProxyQueryParametersMissing(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")

	testURLs := []string{
		"/dns-query",
		"/not-the-right-endpoint",
		"/dns-query?targethost=",
		"/dns-query?targetpath=bar",
	}
	for _, testUrl := range testURLs {
		request, err := http.NewRequest("POST", testUrl, fakeQueryBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, request)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Fatal(fmt.Errorf("failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
		}
		if proxy.lastError != errMissingTargetHost {
			t.Fatal(fmt.Errorf("incorrect error, expected %s", errMissingTargetHost.Error()))
		}
	}

	testURLs = []string{
		"/dns-query?targethost=foo",
		"/dns-query?targethost=foo&targetpath=",
	}
	for _, testUrl := range testURLs {
		request, err := http.NewRequest("POST", testUrl, fakeQueryBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, request)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Fatal(fmt.Errorf("failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
		}
		if proxy.lastError != errMissingTargetPath {
			t.Fatal(fmt.Errorf("incorrect error, expected %s", errMissingTargetPath.Error()))
		}
	}
}

func TestProxyQueryMissingBody(t *testing.T) {
	proxy := proxyServer{}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	emptyQueryBody := strings.NewReader("")
	request, err := http.NewRequest("POST", "/dns-query?targethost=foo&targetpath=bar", emptyQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("failed when sent invalid parameters. Expected %d, got %d", http.StatusBadRequest, status))
	}
	if proxy.lastError != errEmptyRequestBody {
		t.Fatal(fmt.Errorf("incorrect error, expected %s", errEmptyRequestBody.Error()))
	}
}

func TestProxyIncorrectTarget(t *testing.T) {
	proxy := proxyServer{
		client: &http.Client{},
	}

	handler := http.HandlerFunc(proxy.proxyQueryHandler)

	fakeQueryBody := strings.NewReader("test body")
	fakeQueryURL := "/dns-query?targethost=nottherighttarget.com&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Fatal(fmt.Errorf("failed to propagate the desired error code. Expected %d, got %d", http.StatusInternalServerError, status))
	}
}

func TestProxyStatusCodePropagationOK(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprintln(w, "OK")
		if err != nil {
			t.Fatal(err)
		}
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
	fakeQueryURL := "/dns-query" + "?targethost=" + testTargetURL + "&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Fatal(fmt.Errorf("failed to propagate the desired error code. Expected %d, got %d", http.StatusOK, status))
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
	fakeQueryURL := "/dns-query" + "?targethost=" + testTargetURL + "&targetpath=/"

	request, err := http.NewRequest("POST", fakeQueryURL, fakeQueryBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != expectedFailure {
		t.Fatal(fmt.Errorf("failed to propagate the desired error code. Expected %d, got %d", expectedFailure, status))
	}
}
