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
	"fmt"
	"github.com/cloudflare/odoh-go"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func XORBytes(a, b []byte) ([]byte, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("length of byte slices is not equivalent: %d != %d", len(a), len(b))
	}

	buf := make([]byte, len(a))

	for i, _ := range a {
		buf[i] = a[i] ^ b[i]
	}

	return buf, nil
}

func MakeRequest(t *testing.T, target targetServer, encryptedQueryContents []byte) []byte {

	handler := http.HandlerFunc(target.targetQueryHandler)

	request, err := http.NewRequest(http.MethodPost, queryEndpoint, bytes.NewReader(encryptedQueryContents))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Content-Type", odohMessageContentType)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, request)

	cipherText, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}

	odohQueryResponse, err := odoh.UnmarshalDNSMessage(cipherText)
	if err != nil {
		t.Fatal(err)
	}

	return odohQueryResponse.EncryptedMessage
}

func TestProxyReplayAttacks(t *testing.T) {
	var r *localResolver
	r = createLocalResolver(t, 0)
	target := createTarget(t, r)

	plaintextTargetResponses := r.queryResponseMap[r.queries[0]]

	message_difference, _ := XORBytes(plaintextTargetResponses[0], plaintextTargetResponses[1])

	query := r.queries[0]

	obliviousQuery := odoh.CreateObliviousDNSQuery([]byte(query), 0)
	encryptedQuery, _, err := target.odohKeyPair.Config.Contents.EncryptQuery(obliviousQuery)
	if err != nil {
		t.Fatal(err)
	}

	encryptedQueryContents := encryptedQuery.Marshal()

	// Query 1: Client --> Proxy --> Target
	// Proxy doesn't have the context for decrypting the messages and therefore can only perform
	// a replay of the encrypted query (encryptedQuery) to obtain the (odohQueryResponse) from the
	// targets which contains the encrypted response intended for the client.

	// Proxy Maintains the state of available messages during replay
	proxyMessagesSeenInReplay := make(map[string]int)

	// Proxy makes the first request from client and stores `encryptedQueryContents` for a future replay attack.
	cipherText := MakeRequest(t, target, encryptedQueryContents)
	proxyMessagesSeenInReplay[string(cipherText)] = 1

	for {
		// SIMULATION: This is the simulation to indicate that the target has changed the response of the DNS Query.
		cipherTextPossibleChangedResponse := MakeRequest(t, target, encryptedQueryContents)

		if val, ok := proxyMessagesSeenInReplay[string(cipherTextPossibleChangedResponse)]; ok {
			proxyMessagesSeenInReplay[string(cipherTextPossibleChangedResponse)] = val + 1
		} else {
			proxyMessagesSeenInReplay[string(cipherTextPossibleChangedResponse)] = 1
		}

		if len(proxyMessagesSeenInReplay) == 2 {
			break
		}
	}

	fmt.Printf("Total Number of Cipher Texts Seen for a replayed Query : %v\n", len(proxyMessagesSeenInReplay))
	cipherTextResponses := make([][]byte, 0)
	for seenCipherTextResponse, count := range proxyMessagesSeenInReplay {
		fmt.Printf("Response [%v] witnessed [%v] times\n", []byte(seenCipherTextResponse), count)
		cipherTextResponses = append(cipherTextResponses, []byte(seenCipherTextResponse))
	}

	// Proxy can identify
	diff, _ := XORBytes(cipherTextResponses[0], cipherTextResponses[1])
	fmt.Printf("CT DIFF: %v\n", diff)
	fmt.Printf("PT DIFF: %v\n", message_difference)

	if bytes.Contains(diff, message_difference) {
		fmt.Printf("[Replay Attack Successful] Found CT1 xor CT2 equivalent to M1 xor M2 where M1 and M2 are DNS Responses\n")
	}
}
