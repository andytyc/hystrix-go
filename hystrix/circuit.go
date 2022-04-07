package hystrix

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreaker is created for each ExecutorPool to track whether requests
// should be attempted, or rejected if the Health of the circuit is too low.
//
// CircuitBreaker 断路器/电路 | 为每个 ExecutorPool 创建 CircuitBreaker 以跟踪是否应该尝试请求，或者如果电路的 Health 过低则拒绝请求。
type CircuitBreaker struct {
	Name string
	// 开 | 开启熔断
	open bool
	// 强开 | 强制开启熔断
	forceOpen bool
	mutex     *sync.RWMutex
	// 状态变为熔断开 或 允许尝试一次请求处理(之前是熔断开,一直没处理请求) 的最新时间
	openedOrLastTestedTime int64

	// 执行池 | 用于限流
	executorPool *executorPool
	// 指标统计 | 用于熔断
	metrics *metricExchange
}

var (
	// 全局 断路器map锁
	circuitBreakersMutex *sync.RWMutex
	// 全局 断路器map
	circuitBreakers map[string]*CircuitBreaker
)

func init() {
	circuitBreakersMutex = &sync.RWMutex{}
	circuitBreakers = make(map[string]*CircuitBreaker)
}

// GetCircuit returns the circuit for the given command and whether this call created it.
//
// GetCircuit 获取断路器, 没有则创建 | 返回给定命令的电路以及此调用是否创建了它。
func GetCircuit(name string) (*CircuitBreaker, bool, error) {
	circuitBreakersMutex.RLock()
	_, ok := circuitBreakers[name]
	if !ok {
		circuitBreakersMutex.RUnlock()
		circuitBreakersMutex.Lock()
		defer circuitBreakersMutex.Unlock()
		// because we released the rlock before we obtained the exclusive lock,
		// we need to double check that some other thread didn't beat us to
		// creation.
		// 因为我们在获得独占锁之前释放了 rlock，所以我们需要仔细检查其他线程是否没有击败我们创建。
		if cb, ok := circuitBreakers[name]; ok {
			return cb, false, nil
		}
		circuitBreakers[name] = newCircuitBreaker(name)
	} else {
		defer circuitBreakersMutex.RUnlock()
	}

	return circuitBreakers[name], !ok, nil
}

// Flush purges all circuit and metric information from memory.
//
// Flush 释放断路器 | 从内存中清除所有电路和度量信息。
func Flush() {
	circuitBreakersMutex.Lock()
	defer circuitBreakersMutex.Unlock()

	for name, cb := range circuitBreakers {
		cb.metrics.Reset()
		cb.executorPool.Metrics.Reset()
		delete(circuitBreakers, name)
	}
}

// newCircuitBreaker creates a CircuitBreaker with associated Health
//
// newCircuitBreaker 创建断路器 | 创建一个具有关联 Health 的 CircuitBreaker
func newCircuitBreaker(name string) *CircuitBreaker {
	c := &CircuitBreaker{}
	c.Name = name
	c.metrics = newMetricExchange(name)
	c.executorPool = newExecutorPool(name)
	c.mutex = &sync.RWMutex{}

	return c
}

// toggleForceOpen allows manually causing the fallback logic for all instances
// of a given command.
//
// toggleForceOpen 允许为给定命令的所有实例手动触发回退逻辑。
func (circuit *CircuitBreaker) toggleForceOpen(toggle bool) error {
	circuit, _, err := GetCircuit(circuit.Name)
	if err != nil {
		return err
	}

	circuit.forceOpen = toggle
	return nil
}

// IsOpen is called before any Command execution to check whether or
// not it should be attempted. An "open" circuit means it is disabled.
//
// IsOpen 熔断是否为开状态, 如果是则拒绝尝试处理请求 | 在任何命令执行之前调用 IsOpen 以检查是否应该尝试它。 “开”电路意味着它被禁用。
func (circuit *CircuitBreaker) IsOpen() bool {
	circuit.mutex.RLock()
	o := circuit.forceOpen || circuit.open
	circuit.mutex.RUnlock()

	if o {
		return true
	}

	if uint64(circuit.metrics.Requests().Sum(time.Now())) < getSettings(circuit.Name).RequestVolumeThreshold {
		return false
	}

	if !circuit.metrics.IsHealthy(time.Now()) {
		// too many failures, open the circuit
		// 失败太多，熔断打开
		circuit.setOpen()
		return true
	}

	return false
}

// AllowRequest is checked before a command executes, ensuring that circuit state and metric health allow it.
// When the circuit is open, this call will occasionally return true to measure whether the external service
// has recovered.
//
// AllowRequest 查看是否允许尝试处理请求
//
// 在命令执行之前检查 AllowRequest，确保电路状态和度量健康允许它。
// 当电路打开时，这个调用偶尔会返回true来衡量外部服务是否已经恢复。
func (circuit *CircuitBreaker) AllowRequest() bool {
	return !circuit.IsOpen() || circuit.allowSingleTest()
}

// allowSingleTest 允许尝试处理请求 | 即：是否半开状态
func (circuit *CircuitBreaker) allowSingleTest() bool {
	circuit.mutex.RLock()
	defer circuit.mutex.RUnlock()

	now := time.Now().UnixNano()
	openedOrLastTestedTime := atomic.LoadInt64(&circuit.openedOrLastTestedTime)
	if circuit.open && now > openedOrLastTestedTime+getSettings(circuit.Name).SleepWindow.Nanoseconds() {
		// 超出熔断时间段设置，尝试半开: 尝试允许处理请求一次, 如果处理成功了，则会将熔断关闭 -> 可以在 ReportEvent() 函数中看到相关逻辑 | MYDO: 这里是成功一次就熔断关闭，我想可以设置连续成功多少次后才允许熔断关闭
		//
		// CompareAndSwapInt64 将old替换为new, 为了后续的完整性, 交换保证成功 | 返回的就是是否交换成功:swapped
		swapped := atomic.CompareAndSwapInt64(&circuit.openedOrLastTestedTime, openedOrLastTestedTime, now)
		if swapped {
			log.Printf("hystrix-go: allowing single test to possibly close circuit %v", circuit.Name)
		}
		return swapped
	}

	return false
}

// setOpen 熔断打开,并记录打开最新时间(当超过一段时间后，尝试半开，尝试处理请求) | 关 -> 开
func (circuit *CircuitBreaker) setOpen() {
	circuit.mutex.Lock()
	defer circuit.mutex.Unlock()

	if circuit.open {
		return
	}

	log.Printf("hystrix-go: opening circuit %v", circuit.Name)

	circuit.openedOrLastTestedTime = time.Now().UnixNano()
	circuit.open = true
}

// setClose 熔断关闭,并重置统计收集器/计数器 | 开 -> 关
func (circuit *CircuitBreaker) setClose() {
	circuit.mutex.Lock()
	defer circuit.mutex.Unlock()

	if !circuit.open {
		return
	}

	log.Printf("hystrix-go: closing circuit %v", circuit.Name)

	circuit.open = false
	circuit.metrics.Reset()
}

// ReportEvent records command metrics for tracking recent error rates and exposing data to the dashboard.
//
// ReportEvent 请求处理结束进行统计 | 注意：是完毕结束，不代表成功或失败，需要具体来看event | 记录用于跟踪最近错误率和向仪表板公开数据的命令指标。
func (circuit *CircuitBreaker) ReportEvent(eventTypes []string, start time.Time, runDuration time.Duration) error {
	if len(eventTypes) == 0 {
		return fmt.Errorf("no event types sent for metrics")
	}

	circuit.mutex.RLock()
	o := circuit.open
	circuit.mutex.RUnlock()
	if eventTypes[0] == "success" && o { // 注意: 假如 "半开时: 尝试一次处理 allowSingleTest" 成功了, 就直接熔断关闭了. MYDO: 如果以后需要，可以设置为必须连续成功多少次才可以熔断关闭
		circuit.setClose()
	}

	var concurrencyInUse float64
	if circuit.executorPool.Max > 0 {
		concurrencyInUse = float64(circuit.executorPool.ActiveCount()) / float64(circuit.executorPool.Max)
	}

	select {
	case circuit.metrics.Updates <- &commandExecution{
		Types:            eventTypes,
		Start:            start,
		RunDuration:      runDuration,
		ConcurrencyInUse: concurrencyInUse,
	}:
	default:
		return CircuitError{Message: fmt.Sprintf("metrics channel (%v) is at capacity", circuit.Name)}
	}

	return nil
}
