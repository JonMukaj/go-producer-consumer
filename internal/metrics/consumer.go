package metrics

import "github.com/prometheus/client_golang/prometheus"

// ConsumerMetrics holds all Prometheus metrics for the consumer service.
type ConsumerMetrics struct {
	TasksProcessing prometheus.Counter
	TasksDone       prometheus.Counter
	TasksByType     *prometheus.CounterVec
	ValueSumByType  *prometheus.CounterVec
}

// NewConsumerMetrics registers and returns the consumer metrics.
func NewConsumerMetrics(reg prometheus.Registerer) *ConsumerMetrics {
	m := &ConsumerMetrics{
		TasksProcessing: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_processing_total",
			Help: "Total number of tasks moved to processing state.",
		}),
		TasksDone: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_done_total",
			Help: "Total number of tasks completed.",
		}),
		TasksByType: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tasks_by_type_total",
			Help: "Total number of tasks processed per task type.",
		}, []string{"type"}),
		ValueSumByType: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tasks_value_sum_by_type_total",
			Help: "Total sum of task values per task type.",
		}, []string{"type"}),
	}
	reg.MustRegister(
		m.TasksProcessing,
		m.TasksDone,
		m.TasksByType,
		m.ValueSumByType,
	)
	return m
}
