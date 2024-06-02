package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"strconv"
)

var totalRequests = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Number of requests",
	},
	[]string{"path"},
)

var responseStatus = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "response_status",
		Help: "Status of HTTP response",
	},
	[]string{"path", "status"},
)

var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "http_response_time_seconds",
	Help: "Duration of HTTP requests",
}, []string{"path"})

var queueSizeGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "task_queue_size",
	Help: "The size of the task queue",
})

var queueSizeRatioGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "task_queue_size_ratio",
	Help: "The ratio of the queue size",
})

var taskStatusSetToFailedGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "num_tasks_set_to_failed",
	Help: "The number of the tasks set to failed by CheckTaskStatus",
})

func prometheusMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		path := ctx.FullPath()
		timer := prometheus.NewTimer(httpDuration.WithLabelValues(path))
		// Call the next middleware or endpoint handler
		ctx.Next()
		// Update metrics
		totalRequests.WithLabelValues(path).Inc()
		responseStatus.WithLabelValues(path, strconv.Itoa(ctx.Writer.Status())).Inc()
		timer.ObserveDuration()
	}
}
