package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewProducerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewProducerMetrics(reg)
	if m.TasksProduced == nil {
		t.Fatal("TasksProduced counter must not be nil")
	}

	m.TasksProduced.Inc()
	m.TasksProduced.Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "tasks_produced_total" {
			if v := mf.GetMetric()[0].GetCounter().GetValue(); v != 2 {
				t.Errorf("want 2, got %f", v)
			}
			return
		}
	}
	t.Error("tasks_produced_total metric not found")
}

func TestNewConsumerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewConsumerMetrics(reg)

	m.TasksProcessing.Inc()
	m.TasksDone.Inc()
	m.TasksDone.Inc()
	m.TasksByType.WithLabelValues("3").Inc()
	m.ValueSumByType.WithLabelValues("3").Add(42)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	want := map[string]float64{
		"tasks_processing_total":        1,
		"tasks_done_total":              2,
		"tasks_by_type_total":           1,
		"tasks_value_sum_by_type_total": 42,
	}

	for _, mf := range mfs {
		if exp, ok := want[mf.GetName()]; ok {
			got := mf.GetMetric()[0].GetCounter().GetValue()
			if got != exp {
				t.Errorf("%s: want %f, got %f", mf.GetName(), exp, got)
			}
			delete(want, mf.GetName())
		}
	}
	for name := range want {
		t.Errorf("metric %s not found in registry", name)
	}
}
