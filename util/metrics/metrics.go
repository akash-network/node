package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"pkg.akt.dev/go/util/runner"
)

const SuccessLabel = "success"
const FailLabel = "fail"
const OpenLabel = "open"

func IncCounterVecWithLabelValues(counter *prometheus.CounterVec, name string, err error) {
	label := SuccessLabel
	if err != nil {
		label = FailLabel
	}
	counter.WithLabelValues(name, label).Inc()
}

func IncCounterVecWithLabelValuesFiltered(counter *prometheus.CounterVec, name string, err error, filters ...func(error) bool) {
	label := SuccessLabel
	if err != nil {
		flipLabel := true
		for _, filter := range filters {
			if filter(err) {
				flipLabel = false
				break
			}
		}
		if flipLabel {
			label = FailLabel
		}
	}
	counter.WithLabelValues(name, label).Inc()
}

func ObserveRunner(f func() runner.Result, observer prometheus.Histogram) func() runner.Result {
	return func() runner.Result {
		startAt := time.Now()
		result := f()
		elapsed := time.Since(startAt)
		observer.Observe(float64(elapsed.Microseconds()))
		return result
	}
}
