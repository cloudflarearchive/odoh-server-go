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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cloudflare/odoh-go"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

type targetServer struct {
	resolver           resolver
	odohKeyPair        odoh.ObliviousDoHKeyPair
	serverInstanceName string
	experimentId       string
}

const (
	dnsMessageContentType  = "application/dns-message"
	odohMessageContentType = "application/oblivious-dns-message"
)

func decodeDNSQuestion(encodedMessage []byte) (*dns.Msg, error) {
	msg := &dns.Msg{}
	err := msg.Unpack(encodedMessage)
	return msg, err
}

func (s *targetServer) parseQueryFromRequest(r *http.Request) (*dns.Msg, error) {
	switch r.Method {
	case http.MethodGet:
		var queryBody string
		if queryBody = r.URL.Query().Get("dns"); queryBody == "" {
			return nil, fmt.Errorf("missing DNS query parameter in GET request")
		}

		encodedMessage, err := base64.RawURLEncoding.DecodeString(queryBody)
		if err != nil {
			return nil, err
		}

		return decodeDNSQuestion(encodedMessage)
	case http.MethodPost:
		if r.Header.Get("Content-Type") != dnsMessageContentType {
			return nil, fmt.Errorf("incorrect content type, expected '%s', got %s", dnsMessageContentType, r.Header.Get("Content-Type"))
		}

		defer func(body io.ReadCloser) {
			err := body.Close()
			if err != nil {
				log.Warn(err)
			}
		}(r.Body)
		encodedMessage, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}

		return decodeDNSQuestion(encodedMessage)
	default:
		return nil, fmt.Errorf("unsupported HTTP method")
	}
}

func (s *targetServer) resolveQueryWithResolver(q *dns.Msg, r resolver) ([]byte, error) {
	packedQuery, err := q.Pack()
	if err != nil {
		log.Println("Failed encoding DNS query:", err)
		return nil, err
	}

	log.Debugf("Query=%s\n", packedQuery)

	start := time.Now()
	response, err := r.resolve(q)
	if err != nil {
		log.Println("Resolution failed: ", err)
		return nil, err
	}
	elapsed := time.Since(start)

	var packedResponse []byte
	if response != nil {
		packedResponse, err = response.Pack()
		if err != nil {
			log.Warnf("failed encoding DNS response: %s", err)
			return nil, err
		}
	} else {
		errMsg := "empty response from resolver"
		log.Warnf(errMsg)
		return nil, errors.New(errMsg)
	}

	log.Debugf("Answer=%s elapsed=%s\n", packedResponse, elapsed.String())

	return packedResponse, err
}

func (s *targetServer) dohQueryHandler(w http.ResponseWriter, r *http.Request) {
	query, err := s.parseQueryFromRequest(r)
	if err != nil {
		log.Println("Failed parsing request:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	packedResponse, err := s.resolveQueryWithResolver(query, s.resolver)
	if err != nil {
		log.Println("Failed resolving DNS query:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", dnsMessageContentType)
	_, err = w.Write(packedResponse)
	if err != nil {
		log.Warn(err)
	}
}

func (s *targetServer) parseObliviousQueryFromRequest(r *http.Request) (odoh.ObliviousDNSMessage, error) {
	if r.Method != http.MethodPost {
		return odoh.ObliviousDNSMessage{}, fmt.Errorf("unsupported HTTP method for Oblivious DNS query: %s", r.Method)
	}

	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Warn(err)
		}
	}(r.Body)
	encryptedMessageBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return odoh.ObliviousDNSMessage{}, err
	}

	return odoh.UnmarshalDNSMessage(encryptedMessageBytes)
}

func (s *targetServer) createObliviousResponseForQuery(context odoh.ResponseContext, dnsResponse []byte) (odoh.ObliviousDNSMessage, error) {
	response := odoh.CreateObliviousDNSResponse(dnsResponse, 0)
	odohResponse, err := context.EncryptResponse(response)
	if err != nil {
		return odoh.ObliviousDNSMessage{}, err
	}

	log.Debugf("Encrypted response: %x", odohResponse)

	return odohResponse, err
}

func (s *targetServer) odohQueryHandler(w http.ResponseWriter, r *http.Request) {
	odohMessage, err := s.parseObliviousQueryFromRequest(r)
	if err != nil {
		log.Println("parseObliviousQueryFromRequest failed:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	keyID := s.odohKeyPair.Config.Contents.KeyID()
	receivedKeyID := odohMessage.KeyID
	if !bytes.Equal(keyID, receivedKeyID) {
		log.Println("received keyID is different from expected key ID")
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	}

	obliviousQuery, responseContext, err := s.odohKeyPair.DecryptQuery(odohMessage)
	if err != nil {
		log.Println("DecryptQuery failed:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	query, err := decodeDNSQuestion(obliviousQuery.Message())
	if err != nil {
		log.Println("decodeDNSQuestion failed:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	packedResponse, err := s.resolveQueryWithResolver(query, s.resolver)
	if err != nil {
		log.Println("resolveQueryWithResolver failed:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	obliviousResponse, err := s.createObliviousResponseForQuery(responseContext, packedResponse)
	if err != nil {
		log.Println("createObliviousResponseForQuery failed:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	packedResponseMessage := obliviousResponse.Marshal()
	log.Debugf("target response: %x", packedResponseMessage)

	w.Header().Set("Content-Type", odohMessageContentType)
	log.Debug("sending packedResponseMessage")
	_, err = w.Write(packedResponseMessage)
	if err != nil {
		log.Warn(err)
	}
}

func (s *targetServer) targetQueryHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handling target request")

	if r.Header.Get("Content-Type") == dnsMessageContentType {
		s.dohQueryHandler(w, r)
	} else if r.Header.Get("Content-Type") == odohMessageContentType {
		s.odohQueryHandler(w, r)
	} else {
		log.Printf("Invalid content type: %s", r.Header.Get("Content-Type"))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}

func (s *targetServer) configHandler(w http.ResponseWriter, _ *http.Request) {
	configSet := []odoh.ObliviousDoHConfig{s.odohKeyPair.Config}
	configs := odoh.CreateObliviousDoHConfigs(configSet)
	_, err := w.Write(configs.Marshal())
	if err != nil {
		log.Warn(err)
	}
}
