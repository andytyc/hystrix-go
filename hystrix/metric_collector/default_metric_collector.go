package metricCollector

import (
	"sync"

	"github.com/afex/hystrix-go/hystrix/rolling"
)

// DefaultMetricCollector holds information about the circuit state.
// This implementation of MetricCollector is the canonical source of information about the circuit.
// It is used for for all internal hystrix operations
// including circuit health checks and metrics sent to the hystrix dashboard.
//
// Metric Collectors do not need Mutexes as they are updated by circuits within a locked context.
//
// DefaultMetricCollector 保存有关电路状态的信息。
// MetricCollector 的这个实现是关于电路的规范信息源。
// 它用于所有内部 hystrix 操作，包括电路健康检查和发送到 hystrix 仪表板的指标。
//
// 指标/度量收集器不需要互斥锁，因为它们由锁定上下文中的电路更新。
type DefaultMetricCollector struct {
	mutex *sync.RWMutex

	// 请求数量
	numRequests *rolling.Number
	// 错误数量
	errors *rolling.Number

	// 成功数量
	successes *rolling.Number
	// 失败数量
	failures *rolling.Number
	// 拒绝数量
	rejects *rolling.Number
	// 短路数量 | 触发熔断了，不进行尝试请求处理而是直接返回错误
	shortCircuits *rolling.Number
	// 超时数量
	timeouts *rolling.Number
	// ctx取消
	contextCanceled *rolling.Number
	// ctx超时
	contextDeadlineExceeded *rolling.Number

	// 回退成功数量
	fallbackSuccesses *rolling.Number
	// 回退失败数量
	fallbackFailures *rolling.Number
	// 总耗时
	totalDuration *rolling.Timing
	// 运行耗时 | 滚动运行的持续时间
	runDuration *rolling.Timing
}

// newDefaultMetricCollector 创建一个新的指标/度量收集器
func newDefaultMetricCollector(name string) MetricCollector {
	m := &DefaultMetricCollector{}
	m.mutex = &sync.RWMutex{}
	m.Reset()
	return m
}

// NumRequests returns the rolling number of requests
//
// 查询 |
func (d *DefaultMetricCollector) NumRequests() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.numRequests
}

// Errors returns the rolling number of errors
//
// 查询 |
func (d *DefaultMetricCollector) Errors() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.errors
}

// Successes returns the rolling number of successes
//
// 查询 |
func (d *DefaultMetricCollector) Successes() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.successes
}

// Failures returns the rolling number of failures
//
// 查询 |
func (d *DefaultMetricCollector) Failures() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.failures
}

// Rejects returns the rolling number of rejects
//
// 查询 |
func (d *DefaultMetricCollector) Rejects() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.rejects
}

// ShortCircuits returns the rolling number of short circuits
//
// 查询 |
func (d *DefaultMetricCollector) ShortCircuits() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.shortCircuits
}

// Timeouts returns the rolling number of timeouts
//
// 查询 |
func (d *DefaultMetricCollector) Timeouts() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.timeouts
}

// FallbackSuccesses returns the rolling number of fallback successes
//
// 查询 |
func (d *DefaultMetricCollector) FallbackSuccesses() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.fallbackSuccesses
}

// 查询 |
func (d *DefaultMetricCollector) ContextCanceled() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.contextCanceled
}

// ContextDeadlineExceeded ctx超时数量
//
// 查询 |
func (d *DefaultMetricCollector) ContextDeadlineExceeded() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.contextDeadlineExceeded
}

// FallbackFailures returns the rolling number of fallback failures
//
// 查询 |
func (d *DefaultMetricCollector) FallbackFailures() *rolling.Number {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.fallbackFailures
}

// TotalDuration returns the rolling total duration
//
// 查询 | 返回滚动的总持续时间
func (d *DefaultMetricCollector) TotalDuration() *rolling.Timing {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.totalDuration
}

// RunDuration returns the rolling run duration
//
// 查询 | 返回滚动运行的持续时间
func (d *DefaultMetricCollector) RunDuration() *rolling.Timing {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.runDuration
}

// Update 符合 MetricCollector 接口封装
// Update 接受来自远程检测的命令执行的一组指标
func (d *DefaultMetricCollector) Update(r MetricResult) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	d.numRequests.Increment(r.Attempts)
	d.errors.Increment(r.Errors)
	d.successes.Increment(r.Successes)
	d.failures.Increment(r.Failures)
	d.rejects.Increment(r.Rejects)
	d.shortCircuits.Increment(r.ShortCircuits)
	d.timeouts.Increment(r.Timeouts)
	d.fallbackSuccesses.Increment(r.FallbackSuccesses)
	d.fallbackFailures.Increment(r.FallbackFailures)
	d.contextCanceled.Increment(r.ContextCanceled)
	d.contextDeadlineExceeded.Increment(r.ContextDeadlineExceeded)

	d.totalDuration.Add(r.TotalDuration)
	d.runDuration.Add(r.RunDuration)
}

// Reset resets all metrics in this collector to 0.
//
// Reset 符合 MetricCollector 接口封装
// Reset 重置 | 将此收集器中的所有指标重置为 0。
func (d *DefaultMetricCollector) Reset() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.numRequests = rolling.NewNumber()
	d.errors = rolling.NewNumber()
	d.successes = rolling.NewNumber()
	d.rejects = rolling.NewNumber()
	d.shortCircuits = rolling.NewNumber()
	d.failures = rolling.NewNumber()
	d.timeouts = rolling.NewNumber()
	d.fallbackSuccesses = rolling.NewNumber()
	d.fallbackFailures = rolling.NewNumber()
	d.contextCanceled = rolling.NewNumber()
	d.contextDeadlineExceeded = rolling.NewNumber()
	d.totalDuration = rolling.NewTiming()
	d.runDuration = rolling.NewTiming()
}
