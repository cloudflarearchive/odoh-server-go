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
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	mathrand "math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	odoh "github.com/cloudflare/odoh-go"
	"github.com/miekg/dns"
)

type localResolver struct {
	queries          []string
	queryResponseMap map[string][][]byte // Packed DNS queries to responses
}

func (r localResolver) name() string {
	return "localResolver"
}

func (r localResolver) resolve(query *dns.Msg) (*dns.Msg, error) {
	packedQuery, err := query.Pack()
	if err != nil {
		return nil, err
	}

	packed, ok := r.queryResponseMap[string(packedQuery)]
	if !ok {
		return nil, errors.New("Failed to resolve")
	}

	response := &dns.Msg{}
	if len(packed) != 0 {
		randomIndex := mathrand.Intn(len(packed))
		err = response.Unpack(packed[randomIndex])
	}

	return response, err
}

func createLocalResolver(t *testing.T, responseIndex ...int) *localResolver {
	response_messages := []string{"127.0.0.1", "127.0.0.2"}

	if len(responseIndex) != 0 {
		for i, j := 0, len(response_messages)-1; i < j; i, j = i+1, j-1 {
			response_messages[i], response_messages[j] = response_messages[j], response_messages[i]
		}
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	packedQuery, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}

	resultMap := make(map[string][][]byte)
	queries := make([]string, 0)
	queries = append(queries, string(packedQuery))
	responses := make([][]byte, 0)

	for _, response := range response_messages {
		r := new(dns.Msg)
		r.SetReply(q)
		r.Answer = make([]dns.RR, 1)
		r.Answer[0] = &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			A: net.ParseIP(response),
		}
		packedResponse, err := r.Pack()
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, packedResponse)
	}

	resultMap[string(packedQuery)] = responses

	return &localResolver{
		queries:          queries,
		queryResponseMap: resultMap,
	}
}

func createKeyPair(t *testing.T) odoh.ObliviousDoHKeyPair {
	seed := make([]byte, defaultSeedLength)
	rand.Read(seed)

	keyPair, err := odoh.CreateKeyPairFromSeed(kemID, kdfID, aeadID, seed)
	if err != nil {
		t.Fatal("Failed to create a private key. Exiting now.")
	}

	return keyPair
}

func createTarget(t *testing.T, r resolver) targetServer {
	return targetServer{
		resolver:        []resolver{r},
		odohKeyPair:     createKeyPair(t),
		telemetryClient: getTelemetryInstance("LOG"),
	}
}

func TestConfigHandler(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	configSet := []odoh.ObliviousDoHConfig{target.odohKeyPair.Config}
	configs := odoh.CreateObliviousDoHConfigs(configSet)
	marshalledConfig := configs.Marshal()

	handler := http.HandlerFunc(target.configHandler)

	request, err := http.NewRequest("GET", configEndpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Fatal(fmt.Errorf("Failed request with error code: %d", status))
	}

	body, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal("Failed to read body:", err)
	}

	if !bytes.Equal(body, marshalledConfig) {
		t.Fatal("Received invalid config")
	}
}
func TestQueryHandlerInvalidContentType(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	request, err := http.NewRequest("GET", queryEndpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", "application/not-the-droids-youre-looking-for")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusBadRequest, status))
	}
}

func TestQueryHandlerDoHWithPOST(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	request, err := http.NewRequest(http.MethodPost, queryEndpoint, bytes.NewReader([]byte(q)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", dnsMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusOK {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusOK, status))
	}
	if rr.Result().Header.Get("Content-Type") != dnsMessageContentType {
		t.Fatal("Invalid content type response")
	}

	responseBody, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	availableResponses := r.queryResponseMap[q]

	foundMatchResponse := false
	for _, responseFromResponseMap := range availableResponses {
		if bytes.Equal(responseBody, responseFromResponseMap) {
			foundMatchResponse = true
		}
	}

	if foundMatchResponse == false {
		t.Fatal("Incorrect response received")
	}
}

func TestQueryHandlerDoHWithGET(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	encodedQuery := base64.RawURLEncoding.EncodeToString([]byte(q))

	request, err := http.NewRequest(http.MethodGet, queryEndpoint+"?dns="+encodedQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", dnsMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusOK {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusOK, status))
	}
	if rr.Result().Header.Get("Content-Type") != dnsMessageContentType {
		t.Fatal("Invalid content type response")
	}

	responseBody, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	availableResponses := r.queryResponseMap[q]

	foundMatchResponse := false
	for _, responseFromResponseMap := range availableResponses {
		if bytes.Equal(responseBody, responseFromResponseMap) {
			foundMatchResponse = true
		}
	}

	if foundMatchResponse == false {
		t.Fatal("Incorrect response received")
	}
}

func TestQueryHandlerDoHWithInvalidMethod(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	encodedQuery := base64.RawURLEncoding.EncodeToString([]byte(q))
	request, err := http.NewRequest(http.MethodPut, queryEndpoint+"?dns="+encodedQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", dnsMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusBadRequest, status))
	}
}

func TestQueryHandlerODoHWithInvalidMethod(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	obliviousQuery := odoh.CreateObliviousDNSQuery([]byte(q), 0)
	encryptedQuery, _, err := target.odohKeyPair.Config.Contents.EncryptQuery(obliviousQuery)
	if err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodGet, queryEndpoint, bytes.NewReader(encryptedQuery.Marshal()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", odohMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusBadRequest, status))
	}
}

func TestQueryHandlerODoH(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	obliviousQuery := odoh.CreateObliviousDNSQuery([]byte(q), 0)
	encryptedQuery, context, err := target.odohKeyPair.Config.Contents.EncryptQuery(obliviousQuery)
	if err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodPost, queryEndpoint, bytes.NewReader(encryptedQuery.Marshal()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", odohMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusOK {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusOK, status))
	}
	if rr.Result().Header.Get("Content-Type") != odohMessageContentType {
		t.Fatal("Invalid content type response")
	}

	responseBody, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}

	odohQueryResponse, err := odoh.UnmarshalDNSMessage(responseBody)
	if err != nil {
		t.Fatal(err)
	}

	response, err := context.OpenAnswer(odohQueryResponse)
	if err != nil {
		t.Fatal(err)
	}

	availableResponses := r.queryResponseMap[q]

	foundMatchResponse := false
	for _, responseFromResponseMap := range availableResponses {
		if bytes.Equal(response, responseFromResponseMap) {
			foundMatchResponse = true
		}
	}

	if foundMatchResponse == false {
		t.Fatal(fmt.Errorf("Incorrect response received. Got %v \n, expected one of %v", response, r.queryResponseMap[q]))
	}
}

func TestQueryHandlerODoHWithInvalidKey(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	differentKeyPair := createKeyPair(t)
	q := r.queries[0]
	obliviousQuery := odoh.CreateObliviousDNSQuery([]byte(q), 0)
	encryptedQuery, _, err := differentKeyPair.Config.Contents.EncryptQuery(obliviousQuery)
	if err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodPost, queryEndpoint, bytes.NewReader(encryptedQuery.Marshal()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", odohMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusBadRequest, status))
	}
}

func TestQueryHandlerODoHWithCorruptCiphertext(t *testing.T) {
	r := createLocalResolver(t)
	target := createTarget(t, r)

	handler := http.HandlerFunc(target.targetQueryHandler)

	q := r.queries[0]
	obliviousQuery := odoh.CreateObliviousDNSQuery([]byte(q), 0)
	encryptedQuery, _, err := target.odohKeyPair.Config.Contents.EncryptQuery(obliviousQuery)
	if err != nil {
		t.Fatal(err)
	}
	queryBytes := encryptedQuery.Marshal()
	queryBytes[len(queryBytes)-1] ^= 0xFF

	request, err := http.NewRequest(http.MethodPost, queryEndpoint, bytes.NewReader(queryBytes))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", odohMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	if status := rr.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatal(fmt.Errorf("Result did not yield %d, got %d instead", http.StatusBadRequest, status))
	}
}
