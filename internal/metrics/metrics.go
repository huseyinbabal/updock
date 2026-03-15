// Package metrics defines Prometheus metrics for monitoring Updock's behavior.
//
// These metrics are exposed at the /metrics endpoint when metrics are enabled
// (--metrics flag or UPDOCK_METRICS_ENABLED=true). They are compatible with
// standard Prometheus scrape configurations.
//
// # Available Metrics
//
//	updock_containers_scanned      (Gauge)     Number of containers scanned in the last cycle
//	updock_containers_updated_total (Counter)  Total containers successfully updated
//	updock_containers_checked_total (Counter)  Total container update checks performed
//	updock_containers_failed       (Gauge)     Number of containers that failed to update in the last cycle
//	updock_update_errors_total     (Counter)   Total update errors encountered
//	updock_check_duration_seconds  (Histogram) Duration of update check cycles
//	updock_monitored_containers    (Gauge)     Current number of monitored containers
//	updock_scans_total             (Counter)   Total number of scans performed
//
// # Example Prometheus scrape_config
//
//	scrape_configs:
//	  - job_name: updock
//	    scrape_interval: 30s
//	    metrics_path: /metrics
//	    bearer_token: <your-api-token>
//	    static_configs:
//	      - targets: ['updock:8080']
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ContainersScanned reports the number of containers scanned during the
	// most recent update cycle. This is a gauge that resets each cycle.
	ContainersScanned = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "updock",
		Name:      "containers_scanned",
		Help:      "Number of containers scanned for changes during the last scan",
	})

	// ContainersChecked is a monotonically increasing counter of the total
	// number of individual container update checks performed since startup.
	ContainersChecked = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "updock",
		Name:      "containers_checked_total",
		Help:      "Total number of containers checked for updates",
	})

	// ContainersUpdated is a monotonically increasing counter of the total
	// number of containers that have been successfully updated since startup.
	ContainersUpdated = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "updock",
		Name:      "containers_updated_total",
		Help:      "Total number of containers successfully updated",
	})

	// ContainersFailed reports the number of containers where the update failed
	// during the most recent cycle. This is a gauge that resets each cycle.
	ContainersFailed = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "updock",
		Name:      "containers_failed",
		Help:      "Number of containers where update failed during the last scan",
	})

	// UpdateErrors is a monotonically increasing counter of the total number
	// of errors encountered during update attempts since startup.
	UpdateErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "updock",
		Name:      "update_errors_total",
		Help:      "Total number of update errors",
	})

	// CheckDuration records the wall-clock duration of each complete update
	// check cycle in seconds. Uses default histogram buckets.
	CheckDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "updock",
		Name:      "check_duration_seconds",
		Help:      "Duration of update check cycles in seconds",
		Buckets:   prometheus.DefBuckets,
	})

	// MonitoredContainers reports the current number of containers that match
	// the configured monitoring filters.
	MonitoredContainers = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "updock",
		Name:      "monitored_containers",
		Help:      "Current number of monitored containers",
	})

	// ScansTotal is a monotonically increasing counter of the total number of
	// complete scan cycles performed since startup.
	ScansTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "updock",
		Name:      "scans_total",
		Help:      "Total number of scans since updock started",
	})
)
