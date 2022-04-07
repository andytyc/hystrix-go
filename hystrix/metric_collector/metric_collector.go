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
		newDefaultMetricCollector, // 默认的一个, 如果想在自定义添加，调用:Registry.Register()
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
//
// Register 将 MetricCollector Initializer 放置在此 metricCollectorRegistry 维护的注册表中。
func (m *metricCollectorRegistry) Register(initMetricCollector func(string) MetricCollector) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.registry = append(m.registry, initMetricCollector)
}

// MetricResult 指标/度量结果
type MetricResult struct {
	// 尝试
	Attempts float64
	// 错误
	Errors float64
	// 成功
	Successes float64
	// 失败
	Failures float64
	// 拒绝
	Rejects float64
	// 短路 | 触发熔断
	ShortCircuits float64
	// 超时
	Timeouts float64
	// 回退成功
	FallbackSuccesses float64
	// 回退失败
	FallbackFailures float64
	// ctx取消
	ContextCanceled float64
	// ctx超时
	ContextDeadlineExceeded float64
	// 总耗时
	TotalDuration time.Duration
	// 运行耗时
	RunDuration time.Duration
	// 并发使用中
	ConcurrencyInUse float64
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
	// Update 接受来自远程检测的命令执行的一组指标 | 执行统计操作, 如:增量统计、追加时间段
	Update(MetricResult)
	// Reset resets the internal counters and timers.
	//
	// Reset 重置 | 重置内部计数器和定时器。
	Reset()
}
