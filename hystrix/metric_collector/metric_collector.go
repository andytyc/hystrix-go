package metricCollector

import (
	"sync"
	"time"
)

// Registry is the default metricCollectorRegistry that circuits will use to
// collect statistics about the health of the circuit.
//
// Registry 全局 注册登记的指标收集器实例 | 是默认的 metricCollectorRegistry，电路将使用它来收集有关电路健康状况的统计信息。
var Registry = metricCollectorRegistry{
	lock: &sync.RWMutex{},
	registry: []func(name string) MetricCollector{
		newDefaultMetricCollector,
	},
}

type metricCollectorRegistry struct {
	// registry锁
	lock *sync.RWMutex
	// registry 一组: 生成一个"指标/度量收集器"初始化操作的函数逻辑 | 比如: 耗时, 请求数量...
	registry []func(name string) MetricCollector
}

// InitializeMetricCollectors runs the registried MetricCollector Initializers to create an array of MetricCollectors.
//
// InitializeMetricCollectors 初始化 | 运行注册的 MetricCollector Initializers 来创建一个 MetricCollectors 数组。
func (m *metricCollectorRegistry) InitializeMetricCollectors(name string) []MetricCollector {
	m.lock.RLock()
	defer m.lock.RUnlock()

	metrics := make([]MetricCollector, len(m.registry))
	for i, metricCollectorInitializer := range m.registry {
		metrics[i] = metricCollectorInitializer(name)
	}
	return metrics
}

// Register places a MetricCollector Initializer in the registry maintained by this metricCollectorRegistry.
func (m *metricCollectorRegistry) Register(initMetricCollector func(string) MetricCollector) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.registry = append(m.registry, initMetricCollector)
}

type MetricResult struct {
	Attempts                float64
	Errors                  float64
	Successes               float64
	Failures                float64
	Rejects                 float64
	ShortCircuits           float64
	Timeouts                float64
	FallbackSuccesses       float64
	FallbackFailures        float64
	ContextCanceled         float64
	ContextDeadlineExceeded float64
	TotalDuration           time.Duration
	RunDuration             time.Duration
	ConcurrencyInUse        float64
}

// MetricCollector represents the contract that all collectors must fulfill to gather circuit statistics.
// Implementations of this interface do not have to maintain locking around thier data stores so long as
// they are not modified outside of the hystrix context.
//
// MetricCollector 指标/度量收集器 | 用于:统计指标/度量 | 表示所有收集器必须履行以收集电路统计信息的合同。
// 只要不在 hystrix 上下文之外修改此接口的实现，就不必在其数据存储周围保持锁定。
type MetricCollector interface {
	// Update accepts a set of metrics from a command execution for remote instrumentation
	//
	// Update 接受来自远程检测的命令执行的一组指标
	Update(MetricResult)
	// Reset resets the internal counters and timers.
	//
	// Reset 释放资源 | 重置内部计数器和定时器。
	Reset()
}
