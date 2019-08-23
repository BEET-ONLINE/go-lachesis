//go:generate mockgen -package=metrics -self_package=github.com/Fantom-foundation/go-lachesis/lachesis/src/metrics -destination=registry_mock_test.go github.com/Fantom-foundation/go-lachesis/lachesis/src/metrics Registry

package metrics

import (
	"sync"
)

// RegistryEachFunc type for call back function Registry.Each.
type RegistryEachFunc func(name string, metric Metric)

// Registry management metrics.
type Registry interface {
	// Each iteration on all registered metrics.
	Each(f RegistryEachFunc)

	// Register add new metric. If metric is exist, write Fatal error.
	Register(name string, metric Metric)

	// Get the metric by name.
	Get(name string) Metric

	// GetOrRegister getting existing metric or register new metric.
	GetOrRegister(name string, metric Metric) Metric

	// OnNew subscribes f on new Metric registration.
	OnNew(f RegistryEachFunc)
}

var (
	// DefaultRegistry common in memory registry.
	DefaultRegistry = NewRegistry()
)

// NewRegistry constructs a new Registry.
func NewRegistry() Registry {
	return newRegistry()
}

type registry struct {
	*sync.Map
	subscribers []RegistryEachFunc
}

func newRegistry() *registry {
	return &registry{
		new(sync.Map), nil,
	}
}

func (r *registry) Each(f RegistryEachFunc) {
	r.Range(func(key, value interface{}) bool {
		name, ok := key.(string)
		if !ok {
			log.Fatal("name must be string")
		}

		metric, ok := value.(Metric)
		if !ok {
			log.Fatal("metric is incorrect type: must be Metric type")
		}

		f(name, metric)

		return true
	})
}

func (r *registry) Register(name string, metric Metric) {
	_, ok := r.Load(name)
	if ok {
		log.Fatalf("metric '%s' is exist", name)
	}

	r.Store(name, metric)
	r.onNew(name, metric)
}

func (r *registry) Get(name string) Metric {
	value, ok := r.Load(name)
	if !ok {
		return nil
	}

	metric, ok := value.(Metric)
	if !ok {
		return nil
	}

	return metric
}

func (r *registry) GetOrRegister(name string, metric Metric) Metric {
	existingMetric, ok := r.LoadOrStore(name, metric)
	if !ok {
		r.onNew(name, metric)
		return metric
	}

	resultMetric, ok := existingMetric.(Metric)
	if !ok {
		return nil
	}

	return resultMetric
}

func (r *registry) OnNew(f RegistryEachFunc) {
	r.subscribers = append(r.subscribers, f)
	r.Each(f)
}

func (r *registry) onNew(name string, metric Metric) {
	for _, f := range r.subscribers {
		f(name, metric)
	}
}
