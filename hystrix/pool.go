package hystrix

// executorPool 执行池 | 请求需要从令牌池获取令牌 -> 实现服务限流 -> 注意：这里不是采用令牌算法,仅仅是计数器算法
type executorPool struct {
	Name string
	// Metrics 池的相关指标收集器
	Metrics *poolMetrics
	// Max 最大并发量, 一般就是：令牌池的大小
	Max int
	// Tickets 令牌池/凭证池 | 约束服务的处理请求的并发量 | 处理请求前需要获得一个令牌ticket
	Tickets chan *struct{}
}

// newExecutorPool 新建一个执行池，并初始化令牌
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

// Return 回收令牌 | 激活 -> 冷却 | 一般是在请求处理结束的时候调用
func (p *executorPool) Return(ticket *struct{}) {
	if ticket == nil {
		return
	}

	// 更新统计令牌情况
	p.Metrics.Updates <- poolMetricsUpdate{
		activeCount: p.ActiveCount(),
	}
	// 回收令牌 | 激活 -> 冷却
	p.Tickets <- ticket
}

// ActiveCount 激活令牌数,当前已用令牌 | 即: 有多少已经拿了令牌在处理中的请求
func (p *executorPool) ActiveCount() int {
	return p.Max - len(p.Tickets)
}
