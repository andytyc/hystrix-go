package hystrix

import (
	"sync"
	"time"

	metricCollector "github.com/afex/hystrix-go/hystrix/metric_collector"
	"github.com/afex/hystrix-go/hystrix/rolling"
)

// commandExecution 命令执行 | 相关执行状态，执行开始时间...
type commandExecution struct {
	// Types 记录处理请求过程中，所有的相关节点操作细节，比如：哪一步成功，哪一步失败
	//
	// 枚举值 参考:IncrementMetrics()函数中
	Types []string `json:"types"`
	// 处理请求的开始时间, 从收到请求进来就开始了
	Start time.Time `json:"start_time"`
	// runfunC 执行命令操作了，记录此执行命令开始到结束的耗时
	RunDuration time.Duration `json:"run_duration"`
	// 并发使用中 | 即: 当前请求处理中数量 / 设置的最大请求处理量
	ConcurrencyInUse float64 `json:"concurrency_inuse"`
}

// metricExchange 指标交换机 | 同于统计命令执行的过程信息:执行失败,执行中断等, 执行耗时...
type metricExchange struct {
	Name string
	// 命令执行信息统计的通知管道 | 状态变化, 需要统计的相关信息
	Updates chan *commandExecution
	Mutex   *sync.RWMutex

	// 指标/度量收集器
	metricCollectors []metricCollector.MetricCollector
}

// newMetricExchange 新建一个指标交换机，并开启监控
func newMetricExchange(name string) *metricExchange {
	m := &metricExchange{}
	m.Name = name

	m.Updates = make(chan *commandExecution, 2000)
	m.Mutex = &sync.RWMutex{}
	m.metricCollectors = metricCollector.Registry.InitializeMetricCollectors(name)
	m.Reset()

	go m.Monitor()

	return m
}

// The Default Collector function will panic if collectors are not setup to specification.
//
// 如果收集器未按规范设置，默认收集器函数将出现恐慌。
func (m *metricExchange) DefaultCollector() *metricCollector.DefaultMetricCollector {
	if len(m.metricCollectors) < 1 {
		panic("No Metric Collectors Registered.")
	}
	collection, ok := m.metricCollectors[0].(*metricCollector.DefaultMetricCollector)
	if !ok {
		panic("Default metric collector is not registered correctly. The default metric collector must be registered first.")
	}
	return collection
}

// Monitor 开启监控 | 通过 Updates 管道，来监控触发指标收集
func (m *metricExchange) Monitor() {
	for update := range m.Updates {
		// we only grab a read lock to make sure Reset() isn't changing the numbers.
		// 我们只获取一个读锁来确保 Reset() 没有改变数字。
		m.Mutex.RLock()

		totalDuration := time.Since(update.Start)
		wg := &sync.WaitGroup{}
		for _, collector := range m.metricCollectors {
			wg.Add(1)
			go m.IncrementMetrics(wg, collector, update, totalDuration)
		}
		wg.Wait()

		m.Mutex.RUnlock()
	}
}

// IncrementMetrics 增量统计 | 也就是执行统计操作，保存统计信息
func (m *metricExchange) IncrementMetrics(wg *sync.WaitGroup, collector metricCollector.MetricCollector, update *commandExecution, totalDuration time.Duration) {
	// granular metrics
	// 粒度指标
	r := metricCollector.MetricResult{
		Attempts:         1,
		TotalDuration:    totalDuration,
		RunDuration:      update.RunDuration,
		ConcurrencyInUse: update.ConcurrencyInUse,
	}

	switch update.Types[0] {
	case "success":
		r.Successes = 1
	case "failure":
		r.Failures = 1
		r.Errors = 1
	case "rejected":
		r.Rejects = 1
		r.Errors = 1
	case "short-circuit":
		r.ShortCircuits = 1
		r.Errors = 1
	case "timeout":
		r.Timeouts = 1
		r.Errors = 1
	case "context_canceled":
		r.ContextCanceled = 1
	case "context_deadline_exceeded":
		r.ContextDeadlineExceeded = 1
	}

	if len(update.Types) > 1 {
		// fallback metrics
		// 回退指标
		if update.Types[1] == "fallback-success" {
			r.FallbackSuccesses = 1
		}
		if update.Types[1] == "fallback-failure" {
			r.FallbackFailures = 1
		}
	}

	collector.Update(r)

	wg.Done()
}

// Reset 重置 | 重置指标收集器
func (m *metricExchange) Reset() {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	for _, collector := range m.metricCollectors {
		collector.Reset()
	}
}

// Requests 查询 | 请求数
func (m *metricExchange) Requests() *rolling.Number {
	m.Mutex.RLock()
	defer m.Mutex.RUnlock()
	return m.requestsLocked()
}

func (m *metricExchange) requestsLocked() *rolling.Number {
	return m.DefaultCollector().NumRequests()
}

// ErrorPercent 查询 | 错误/请求 = 错误的百分比
func (m *metricExchange) ErrorPercent(now time.Time) int {
	m.Mutex.RLock()
	defer m.Mutex.RUnlock()

	var errPct float64
	reqs := m.requestsLocked().Sum(now)
	errs := m.DefaultCollector().Errors().Sum(now)

	if reqs > 0 {
		errPct = (float64(errs) / float64(reqs)) * 100
	}

	return int(errPct + 0.5)
}

// IsHealthy 是否健康, ok 则允许尝试处理请求 | 将错误处理的百分比 和 断路器配置的熔断条件相比较
func (m *metricExchange) IsHealthy(now time.Time) bool {
	return m.ErrorPercent(now) < getSettings(m.Name).ErrorPercentThreshold
}
