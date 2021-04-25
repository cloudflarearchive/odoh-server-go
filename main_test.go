package main

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestGenKeyPair(t *testing.T) {
	_, err := keyPair()
	if err != nil {
		t.Error(err)
	}
}

func TestHTTPServer(t *testing.T) {
	kp, err := keyPair()
	if err != nil {
		t.Error(err)
	}
	proxy, target := serverPair(*kp)
	setupHandlers(proxy, target)
	go serve("127.0.0.1:8081", false)

	// Wait for server startup
	time.Sleep(time.Second)

	httpClient := http.Client{}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8081/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "ok" {
		t.Fatalf("healthcheck response mismatch, expected \"ok\" got \"%s\"", body)
	}
}
