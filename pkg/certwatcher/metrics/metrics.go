package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/krateoplatformops/unstructured-runtime/pkg/metrics"
)

var (
	// ReadCertificateTotal is a prometheus counter metrics which holds the total
	// number of certificate reads.
	ReadCertificateTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "certwatcher_read_certificate_total",
		Help: "Total number of certificate reads",
	})

	// ReadCertificateErrors is a prometheus counter metrics which holds the total
	// number of errors from certificate read.
	ReadCertificateErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "certwatcher_read_certificate_errors_total",
		Help: "Total number of certificate read errors",
	})
)

func init() {
	metrics.Registry.MustRegister(
		ReadCertificateTotal,
		ReadCertificateErrors,
	)
}
