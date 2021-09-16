package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	kp, err := keyPair()
	if err != nil {
		panic(err)
	}

	// Disable TLS strict certificate checking for local self signed cert
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	proxy, target := serverPair(*kp)
	setupHandlers(proxy, target)

	os.Exit(m.Run())
}

func TestGenKeyPair(t *testing.T) {
	_, err := keyPair()
	assert.Nil(t, err)
}

func TestServeHTTP(t *testing.T) {
	go serve("127.0.0.1:8080", false, "", "")

	// Wait for server startup
	time.Sleep(50 * time.Millisecond)

	httpClient := http.Client{}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8080/health", nil)
	assert.Nil(t, err)

	resp, err := httpClient.Do(req)
	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	assert.Equal(t, "ok", string(body))
}

func TestServeHTTPS(t *testing.T) {
	go serve("127.0.0.1:8443", true, "cert.pem", "key.pem")

	// Wait for server startup
	time.Sleep(50 * time.Millisecond)

	httpClient := http.Client{}
	req, err := http.NewRequest("GET", "https://127.0.0.1:8443/health", nil)
	assert.Nil(t, err)

	resp, err := httpClient.Do(req)
	assert.Nil(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	assert.Equal(t, "ok", string(body))
}
