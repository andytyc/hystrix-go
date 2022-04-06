package hystrix

// executorPool 令牌池 -> 实现服务限流 -> 采用令牌算法
type executorPool struct {
	Name    string
	Metrics *poolMetrics
	// Max 最大并发量, 一般就是：令牌池的大小
	Max int
	// Tickets 令牌池
	Tickets chan *struct{}
}

func newExecutorPool(name string) *executorPool {
	p := &executorPool{}
	p.Name = name
	p.Metrics = newPoolMetrics(name)
	p.Max = getSettings(name).MaxConcurrentRequests

	p.Tickets = make(chan *struct{}, p.Max)
	for i := 0; i < p.Max; i++ {
		p.Tickets <- &struct{}{}
	}

	return p
}

// Return 回收令牌(激活 -> 冷却) | 一般是在请求处理结束的时候调用
func (p *executorPool) Return(ticket *struct{}) {
	if ticket == nil {
		return
	}

	// 更新统计
	p.Metrics.Updates <- poolMetricsUpdate{
		activeCount: p.ActiveCount(),
	}
	p.Tickets <- ticket
}

// ActiveCount 当前已用令牌(激活)
func (p *executorPool) ActiveCount() int {
	return p.Max - len(p.Tickets)
}
