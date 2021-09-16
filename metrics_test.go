package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetricsServe(t *testing.T) {
	go func() {
		err := metricsServe("127.0.0.1:8081")
		assert.Nil(t, err)
	}()

	// Wait for server startup
	time.Sleep(50 * time.Millisecond)

	httpClient := http.Client{}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8081/metrics", nil)
	assert.Nil(t, err)

	resp, err := httpClient.Do(req)
	assert.Nil(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}
