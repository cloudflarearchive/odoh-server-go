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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	odoh "github.com/cloudflare/odoh-go"
	"github.com/miekg/dns"
)

type localResolver struct {
	queryResponseMap map[string][]byte // Packed DNS queries to responses
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
	err = response.Unpack(packed)

	return response, err
}

func TestConfigHandler(t *testing.T) {
	seed := make([]byte, defaultSeedLength)
	rand.Read(seed)

	keyPair, err := odoh.CreateKeyPairFromSeed(kemID, kdfID, aeadID, seed)
	if err != nil {
		t.Fatal("Failed to create a private key. Exiting now.")
	}

	configSet := []odoh.ObliviousDoHConfig{keyPair.Config}
	configs := odoh.CreateObliviousDoHConfigs(configSet)
	marshalledConfig := configs.Marshal()

	target := targetServer{
		resolver:    []resolver{&localResolver{}},
		odohKeyPair: keyPair,
	}

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
