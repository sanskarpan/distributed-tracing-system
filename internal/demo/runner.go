package demo

import (
	"log"
	"math/rand"
	"time"
)

// Runner continuously generates traces by running weighted random scenarios.
type Runner struct {
	collectorURL string
	stopCh       chan struct{}
	totalWeight  int
}

// NewRunner creates a new demo runner pointing at the given collector URL.
func NewRunner(collectorURL string) *Runner {
	total := 0
	for _, s := range Scenarios {
		total += s.Weight
	}
	return &Runner{
		collectorURL: collectorURL,
		stopCh:       make(chan struct{}),
		totalWeight:  total,
	}
}

// Start begins generating traces in the background.
func (r *Runner) Start() {
	go r.run()
}

func (r *Runner) run() {
	for {
		scenario := r.pickScenario()
		sdk := NewDemoSDK("demo", r.collectorURL)

		if err := scenario.Run(sdk); err != nil {
			log.Printf("scenario %s: %v", scenario.Name, err)
		}

		sleepMs := 500 + rand.Intn(2500)
		select {
		case <-r.stopCh:
			return
		case <-time.After(time.Duration(sleepMs) * time.Millisecond):
		}
	}
}

func (r *Runner) pickScenario() Scenario {
	n := rand.Intn(r.totalWeight)
	cumulative := 0
	for _, s := range Scenarios {
		cumulative += s.Weight
		if n < cumulative {
			return s
		}
	}
	return Scenarios[0]
}

// Stop signals the runner to stop after the current scenario finishes.
func (r *Runner) Stop() {
	close(r.stopCh)
}
