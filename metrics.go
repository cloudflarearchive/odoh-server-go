package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricTargetQueries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "odohd_target_queries",
		Help: "Total queries as a target",
	})
	metricProxyQueries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "odohd_proxy_queries",
		Help: "Total queries as a proxy",
	})
	metricTargetValidQueries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "odohd_target_valid_queries",
		Help: "Total valid queries as a target",
	})
	metricProxyValidQueries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "odohd_proxy_valid_queries",
		Help: "Total valid queries as a proxy",
	})
)

// metricsServe starts the metrics HTTP server
func metricsServe(listenAddr string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(listenAddr, nil)
}
