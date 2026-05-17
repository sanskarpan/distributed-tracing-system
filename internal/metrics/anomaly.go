package metrics

import "math"

// AnomalyResult describes a latency outlier for a (service, operation) pair.
type AnomalyResult struct {
	Service   string  `json:"service"`
	Operation string  `json:"operation"`
	P99Ms     float64 `json:"p99Ms"`
	MeanMs    float64 `json:"meanMs"`
	StddevMs  float64 `json:"stddevMs"`
	ZScore    float64 `json:"zScore"`
	IsOutlier bool    `json:"isOutlier"`
}

// DetectAnomalies scans all operation snapshots and flags those whose P99 latency
// is more than zThreshold standard deviations above the population mean.
func (m *MetricsStore) DetectAnomalies(zThreshold float64, tenantID string) []AnomalyResult {
	snapshots := m.Snapshot(tenantID)
	if len(snapshots) == 0 {
		return nil
	}

	p99s := make([]float64, len(snapshots))
	for i, s := range snapshots {
		p99s[i] = s.P99Ms
	}

	mean := computeMean(p99s)
	stddev := computeStddev(p99s, mean)

	results := make([]AnomalyResult, 0, len(snapshots))
	for _, s := range snapshots {
		var z float64
		if stddev > 0 {
			z = (s.P99Ms - mean) / stddev
		}
		results = append(results, AnomalyResult{
			Service:   s.Service,
			Operation: s.Operation,
			P99Ms:     s.P99Ms,
			MeanMs:    mean,
			StddevMs:  stddev,
			ZScore:    z,
			IsOutlier: z > zThreshold,
		})
	}
	return results
}

func computeMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func computeStddev(vals []float64, mean float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	variance := 0.0
	for _, v := range vals {
		d := v - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(vals)))
}
