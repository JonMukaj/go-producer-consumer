package metrics

import "github.com/prometheus/client_golang/prometheus"

// ProducerMetrics holds all Prometheus metrics for the producer service.
type ProducerMetrics struct {
	TasksProduced prometheus.Counter
}

// NewProducerMetrics registers and returns the producer metrics.
func NewProducerMetrics(reg prometheus.Registerer) *ProducerMetrics {
	m := &ProducerMetrics{
		TasksProduced: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_produced_total",
			Help: "Total number of tasks produced and sent to the consumer.",
		}),
	}
	reg.MustRegister(m.TasksProduced)
	return m
}
