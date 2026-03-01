package metrics

import "github.com/prometheus/client_golang/prometheus"

type SecretMetrics struct {
	SecretsCreated   prometheus.Counter
	SecretsRetrieved prometheus.Counter
	SecretsDeleted   *prometheus.CounterVec
	CleanupErrors    prometheus.Counter
}

func NewSecretMetrics(reg *prometheus.Registry) *SecretMetrics {
	m := &SecretMetrics{
		SecretsCreated: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "secrets_created_total",
				Help:      "Total number of secrets created.",
			},
		),
		SecretsRetrieved: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "secrets_retrieved_total",
				Help:      "Total number of secrets successfully retrieved.",
			},
		),
		SecretsDeleted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "secrets_deleted_total",
				Help:      "Total number of secrets deleted, by method (api, burn, cleanup).",
			},
			[]string{"method"},
		),
		CleanupErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cleanup_errors_total",
				Help:      "Total number of errors encountered by the cleanup worker (DB or S3).",
			},
		),
	}
	reg.MustRegister(m.SecretsCreated, m.SecretsRetrieved, m.SecretsDeleted, m.CleanupErrors)
	return m
}
