package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	labelForYunApi    = []string{"pluginName", "type", "action", "diskId", "returnCode"}
	labelForOperation = []string{"pluginName", "action", "diskId", "nodeName", "returnCode"}
	buckets           = []float64{0.1, 0.5, 1, 5, 8, 10, 12, 15}

	YunApiRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			// Namespace: common.QcloudCbsPluginName,
			Name: "cbs_plugin_yunapi_request_total",
			Help: "QcloudCbs' yunapi request count",
		}, labelForYunApi)

	// errors count for attach/detach
	OperationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			// Namespace: common.QcloudCbsPluginName,
			Name: "cbs_plugin_operation_errors_total",
			Help: "QcloudCbs' operation(attach/detach) errors",
		}, labelForOperation)

	YunApiRequestCostSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cbs_plugin_yunapi_request_cost_seconds",
			Help:    "Latency of Tencent YunApi calls",
			Buckets: buckets,
		}, []string{"pluginName", "type", "action"})

	// count for provision/delete
	CbsPvcsRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			// Namespace: common.QcloudCbsPluginName,
			// Subsystem: "cbs-provisioner",
			Name: "cbs_pvc_request_total",
			Help: "total requests count of provision or delete cbs pvcs",
		}, []string{"pluginName", "action", "returnCode"})

	DevicePathNotExist = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cbs_plugin_devicepath_not_exist_total",
			Help: "total count of devicepath(/dev/disk/by-id/virtio-disk-xxx) not exist",
		}, []string{"pluginName"})
)

var registerOnce sync.Once

func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(YunApiRequestTotal)
		prometheus.MustRegister(YunApiRequestCostSeconds)
		prometheus.MustRegister(OperationErrorsTotal)
		prometheus.MustRegister(CbsPvcsRequestTotal)
		prometheus.MustRegister(DevicePathNotExist)
	})
}
