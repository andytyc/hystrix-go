package hystrix

import (
	"sync"

	"github.com/afex/hystrix-go/hystrix/rolling"
)

// poolMetrics 池指标/度量收集器 | 用来收集执行池的操作相关指标统计
type poolMetrics struct {
	Mutex *sync.RWMutex
	// 有需要更新的统计数据
	Updates chan poolMetricsUpdate

	Name string
	// 令牌激活数量,当前已用令牌的最大值,峰值 | 即: 有多少已经拿了令牌在处理中的请求 | 指标统计
	MaxActiveRequests *rolling.Number
	// 统计Update操作执行次数 | 指标统计
	Executed *rolling.Number
}

type poolMetricsUpdate struct {
	// 令牌激活数量,当前已用令牌 | 即: 有多少已经拿了令牌在处理中的请求
	activeCount int
}

// newPoolMetrics 新建一个池指标收集器，并开启监控
func newPoolMetrics(name string) *poolMetrics {
	m := &poolMetrics{}
	m.Name = name
	m.Updates = make(chan poolMetricsUpdate)
	m.Mutex = &sync.RWMutex{}

	m.Reset()

	go m.Monitor()

	return m
}

// Reset 重置统计计数器
func (m *poolMetrics) Reset() {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	m.MaxActiveRequests = rolling.NewNumber()
	m.Executed = rolling.NewNumber()
}

// Monitor 开启监控 | 管道通知触发监控任务，进行统计信息操作
func (m *poolMetrics) Monitor() {
	for u := range m.Updates {
		m.Mutex.RLock()

		m.Executed.Increment(1)
		m.MaxActiveRequests.UpdateMax(float64(u.activeCount))

		m.Mutex.RUnlock()
	}
}
