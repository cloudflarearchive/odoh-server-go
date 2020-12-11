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
	"cloud.google.com/go/logging"
	"context"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"log"
	"net/http"
	"strings"
	"sync"
)

// This runningTime structure contains the epoch timestamps for the following operations
// 1. Start => Epoch time at which the request is received by the ObliviousDNSHandler
// 2. TargetQueryDecryptionTime => Epoch
type runningTime struct {
	Start                      int64
	TargetQueryDecryptionTime  int64
	TargetQueryResolutionTime  int64
	TargetAnswerEncryptionTime int64
	EndTime                    int64
}

type experiment struct {
	RequestID    []byte
	Resolver     string
	Timestamp    runningTime
	Status       bool
	IngestedFrom string
	ExperimentID string
	ProtocolType string
}

func (e *experiment) serialize() string {
	exp := &e
	response, err := json.Marshal(exp)
	if err != nil {
		log.Printf("Unable to log the information correctly.")
	}
	return string(response)
}

type telemetry struct {
	sync.RWMutex
	esClient    *elasticsearch.Client
	buffer      []string
	logClient   *logging.Client
	cloudlogger *logging.Logger
}

const (
	INDEX = "server_telemetry"
	TYPE  = "client_localhost"
)

var telemetryInstance telemetry

func getTelemetryInstance(telemetryType string) *telemetry {
	var err error
	if telemetryType == "ELK" {
		elasticsearchTransport := elasticsearch.Config{
			Addresses: []string{
				"http://localhost:9200",
			},
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
			},
		}
		telemetryInstance.esClient, err = elasticsearch.NewClient(elasticsearchTransport)
		if err != nil {
			log.Fatalf("Unable to create an elasticsearch client connection.")
		}
	} else if telemetryType == "GCP" {
		ctx := context.Background()
		projectID := "odoh-target"
		telemetryInstance.logClient, err = logging.NewClient(ctx, projectID)
		if err != nil {
			log.Fatalf("Unable to create a logging instance to Google Cloud %v", err)
		}
		logName := "odohserver-gcp"
		telemetryInstance.cloudlogger = telemetryInstance.logClient.Logger(logName)
	} else {
		telemetryInstance.cloudlogger = nil
		telemetryInstance.esClient = nil
	}
	return &telemetryInstance
}

func (t *telemetry) streamTelemetryToGCPLogging(dataItems []string) {
	defer t.cloudlogger.Flush()
	for _, item := range dataItems {
		log.Printf("Logging %v to the GCP instance\n", item)
		t.cloudlogger.Log(logging.Entry{Payload: item})
	}
}

func (t *telemetry) streamDataToElastic(dataItems []string) {
	var wg sync.WaitGroup
	for index, item := range dataItems {
		wg.Add(1)
		go func(i int, message string) {
			defer wg.Done()
			req := esapi.IndexRequest{
				Index:   INDEX,
				Body:    strings.NewReader(message),
				Refresh: "true",
			}

			res, err := req.Do(context.Background(), t.esClient)
			if err != nil {
				log.Printf("Unable to send the request to elastic.")
			}
			defer res.Body.Close()
			if res.IsError() {
				log.Printf("[%s] Error Indexing Value [%s]", res.Status(), message)
			} else {
				log.Printf("Successfully Inserted [%s]", message)
			}
		}(index, item)
	}
	wg.Wait()
}
