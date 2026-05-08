package metrics

// SLOResult holds the error budget status for a service.
type SLOResult struct {
	Service          string  `json:"service"`
	// TargetErrorRate is the maximum acceptable error ratio (e.g. 0.01 = 1%)
	TargetErrorRate  float64 `json:"targetErrorRate"`
	// CurrentErrorRate is the actual error ratio in the current window
	CurrentErrorRate float64 `json:"currentErrorRate"`
	// BudgetRemaining is the fraction of error budget still available (0–1)
	BudgetRemaining  float64 `json:"budgetRemaining"`
	// Breached is true when the current error rate exceeds the SLO target
	Breached         bool    `json:"breached"`
}

// ComputeSLOs aggregates error rates per service and computes error budget
// against the given target error rate. targetErrorRate is applied globally
// across all services (e.g. 0.01 = 99% availability SLO).
func (m *MetricsStore) ComputeSLOs(targetErrorRate float64) []SLOResult {
	snapshots := m.Snapshot()

	// Aggregate per service: weighted sum of requests and errors
	type svcStats struct {
		totalRate float64
		errorRate float64
	}
	svcMap := make(map[string]*svcStats)
	for _, s := range snapshots {
		st, ok := svcMap[s.Service]
		if !ok {
			st = &svcStats{}
			svcMap[s.Service] = st
		}
		st.totalRate += s.Rate
		st.errorRate += s.Rate * s.ErrorRate
	}

	results := make([]SLOResult, 0, len(svcMap))
	for svc, st := range svcMap {
		var currentErrRate float64
		if st.totalRate > 0 {
			currentErrRate = st.errorRate / st.totalRate
		}

		var budgetRemaining float64
		if targetErrorRate > 0 {
			budgetRemaining = 1.0 - (currentErrRate / targetErrorRate)
			if budgetRemaining < 0 {
				budgetRemaining = 0
			}
			if budgetRemaining > 1 {
				budgetRemaining = 1
			}
		}

		results = append(results, SLOResult{
			Service:         svc,
			TargetErrorRate: targetErrorRate,
			CurrentErrorRate: currentErrRate,
			BudgetRemaining: budgetRemaining,
			Breached:        currentErrRate > targetErrorRate,
		})
	}
	return results
}
