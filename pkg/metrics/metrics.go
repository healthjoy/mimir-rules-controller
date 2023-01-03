package metrics

import (
	"context"
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Metrics is a wrapper around the prometheus registry
type Metrics struct {
	Registry *prometheus.Registry
}

// New creates a new metrics server with the default collectors
func New() *Metrics {
	reg := metrics.Registry
	// Register some standard metrics
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewBuildInfoCollector())

	return &Metrics{Registry: reg.(*prometheus.Registry)}
}

// Start starts a metrics server on the given address
func (m *Metrics) Start(ctx context.Context, addr string) {
	go m.runServer(ctx, addr)
}

func (m *Metrics) runServer(ctx context.Context, addr string) {

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ok\n"))
	}))
	mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ok\n"))
	}))
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				runtime.HandleError(err)
				klog.Fatal(err)
			}
		}
	}()
	<-ctx.Done()
	if err := server.Shutdown(ctx); err != nil {
		runtime.HandleError(err)
	}
}
