package hystrix

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// runFunc 执行命令
type runFunc func() error

// fallbackFunc 执行命令失败后，回退命令
type fallbackFunc func(error) error

// runFunc 执行命令 ctx
type runFuncC func(context.Context) error

// fallbackFunc 执行命令失败后，回退命令 ctx
type fallbackFuncC func(context.Context, error) error

// A CircuitError is an error which models various failure states of execution,
// such as the circuit being open or a timeout.
// CircuitError 是模拟各种执行失败状态的错误，
// 如电路开路或超时。
type CircuitError struct {
	Message string
}

func (e CircuitError) Error() string {
	return "hystrix: " + e.Message
}

// command models the state used for a single execution on a circuit. "hystrix command" is commonly
// used to describe the pairing of your run/fallback functions with a circuit.
// command 模拟用于电路上单次执行的状态。“hystrix 命令”通常是
// 用于描述运行/回退功能与电路的配对。
type command struct {
	sync.Mutex

	ticket   *struct{}
	start    time.Time
	errChan  chan error
	finished chan bool
	// 断路器
	circuit *CircuitBreaker
	// 执行命令
	run runFuncC
	// 回退命令 执行错误时触发
	fallback    fallbackFuncC
	runDuration time.Duration
	events      []string
}

var (
	// ErrMaxConcurrency occurs when too many of the same named command are executed at the same time.
	// ErrMaxConcurrency 当太多同名命令同时执行时发生。
	ErrMaxConcurrency = CircuitError{Message: "max concurrency"}
	// ErrCircuitOpen returns when an execution attempt "short circuits". This happens due to the circuit being measured as unhealthy.
	// ErrCircuitOpen 在执行尝试“短路”时返回。这是由于电路被测量为不健康而发生的。
	ErrCircuitOpen = CircuitError{Message: "circuit open"}
	// ErrTimeout occurs when the provided function takes too long to execute.
	// ErrTimeout 在提供的函数执行时间过长时发生。
	ErrTimeout = CircuitError{Message: "timeout"}
)

// Go runs your function while tracking the health of previous calls to it.
// If your function begins slowing down or failing repeatedly, we will block
// new calls to it for you to give the dependent service time to repair.
//
// Define a fallback function if you want to define some code to execute during outages.
//
// Go 运行你的函数，同时跟踪之前调用它的运行状况。
// 如果你的函数开始变慢或反复失败，我们将阻塞
// 新调用它，让您给依赖服务时间修复。
//
// 如果您想定义一些代码在中断期间执行，请定义一个回退函数。
func Go(name string, run runFunc, fallback fallbackFunc) chan error {
	runC := func(ctx context.Context) error {
		return run()
	}
	var fallbackC fallbackFuncC
	if fallback != nil {
		fallbackC = func(ctx context.Context, err error) error {
			return fallback(err)
		}
	}
	return GoC(context.Background(), name, runC, fallbackC)
}

// GoC runs your function while tracking the health of previous calls to it.
// If your function begins slowing down or failing repeatedly, we will block
// new calls to it for you to give the dependent service time to repair.
//
// Define a fallback function if you want to define some code to execute during outages.
//
// GoC 运行您的函数，同时跟踪之前对其调用的健康状况。
// 如果你的函数开始变慢或反复失败，我们将阻塞
// 新调用它，让您给依赖服务时间修复。
//
// 如果您想定义一些代码在中断期间执行，请定义一个回退函数。
//
// GoC 相对于Go 添加了上下文
func GoC(ctx context.Context, name string, run runFuncC, fallback fallbackFuncC) chan error {
	// MYDO: cmd可以对象复用, 优化内存
	cmd := &command{
		run:      run,
		fallback: fallback,
		start:    time.Now(),
		errChan:  make(chan error, 1),
		finished: make(chan bool, 1),
	}

	// dont have methods with explicit params and returns
	// let data come in and out naturally, like with any closure
	// explicit error return to give place for us to kill switch the operation (fallback)

	// 没有带有显式参数和返回的方法
	// 让数据自然地进出，就像任何闭包一样
	// 显式错误返回给我们杀死开关操作的地方（回退）

	circuit, _, err := GetCircuit(name)
	if err != nil {
		cmd.errChan <- err
		return cmd.errChan
	}
	cmd.circuit = circuit

	ticketCond := sync.NewCond(cmd)
	ticketChecked := false
	// When the caller extracts error from returned errChan, it's assumed that
	// the ticket's been returned to executorPool. Therefore, returnTicket() can
	// not run after cmd.errorWithFallback().
	// 当调用者从返回的 errChan 中提取错误时，假设票证已经返回到 executorPool。
	// 因此，在 cmd.errorWithFallback() 之后，returnTicket() 无法运行。
	returnTicket := func() {
		cmd.Lock()
		// Avoid releasing before a ticket is acquired.
		// 避免在获取票证之前释放。
		for !ticketChecked {
			ticketCond.Wait()
		}
		cmd.circuit.executorPool.Return(cmd.ticket)
		cmd.Unlock()
	}
	// Shared by the following two goroutines. It ensures only the faster
	// goroutine runs errWithFallback() and reportAllEvent().
	// 由以下两个 goroutine 共享。
	// 它确保只有更快的 goroutine 运行 errWithFallback() 和 reportAllEvent()。
	returnOnce := &sync.Once{}
	reportAllEvent := func() {
		err := cmd.circuit.ReportEvent(cmd.events, cmd.start, cmd.runDuration)
		if err != nil {
			log.Printf(err.Error())
		}
	}

	go func() {
		defer func() { cmd.finished <- true }()

		// Circuits get opened when recent executions have shown to have a high error rate.
		// Rejecting new executions allows backends to recover, and the circuit will allow
		// new traffic when it feels a healthly state has returned.
		if !cmd.circuit.AllowRequest() {
			cmd.Lock()
			// It's safe for another goroutine to go ahead releasing a nil ticket.
			ticketChecked = true
			ticketCond.Signal()
			cmd.Unlock()
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ErrCircuitOpen)
				reportAllEvent()
			})
			return
		}

		// As backends falter, requests take longer but don't always fail.
		//
		// When requests slow down but the incoming rate of requests stays the same, you have to
		// run more at a time to keep up. By controlling concurrency during these situations, you can
		// shed load which accumulates due to the increasing ratio of active commands to incoming requests.
		cmd.Lock()
		select {
		case cmd.ticket = <-circuit.executorPool.Tickets:
			// 获取到令牌，可以执行命令
			ticketChecked = true
			ticketCond.Signal()
			cmd.Unlock()
		default:
			// 没有令牌则，拒绝执行命令
			ticketChecked = true
			ticketCond.Signal()
			cmd.Unlock()
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ErrMaxConcurrency)
				reportAllEvent()
			})
			return
		}

		runStart := time.Now()
		runErr := run(ctx)
		returnOnce.Do(func() {
			defer reportAllEvent()
			cmd.runDuration = time.Since(runStart)
			returnTicket()
			if runErr != nil {
				cmd.errorWithFallback(ctx, runErr)
				return
			}
			cmd.reportEvent("success")
		})
	}()

	go func() {
		timer := time.NewTimer(getSettings(name).Timeout)
		defer timer.Stop()

		select {
		case <-cmd.finished:
			// returnOnce has been executed in another goroutine
		case <-ctx.Done():
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ctx.Err())
				reportAllEvent()
			})
			return
		case <-timer.C:
			returnOnce.Do(func() {
				returnTicket()
				cmd.errorWithFallback(ctx, ErrTimeout)
				reportAllEvent()
			})
			return
		}
	}()

	return cmd.errChan
}

// Do runs your function in a synchronous manner, blocking until either your function succeeds
// or an error is returned, including hystrix circuit errors
// Do 以同步方式运行你的函数，阻塞直到你的函数成功或返回错误，包括 hystrix 电路错误
func Do(name string, run runFunc, fallback fallbackFunc) error {
	runC := func(ctx context.Context) error {
		return run()
	}
	var fallbackC fallbackFuncC
	if fallback != nil {
		fallbackC = func(ctx context.Context, err error) error {
			return fallback(err)
		}
	}
	return DoC(context.Background(), name, runC, fallbackC)
}

// DoC runs your function in a synchronous manner, blocking until either your function succeeds
// or an error is returned, including hystrix circuit errors
// DoC 以同步方式运行你的函数，阻塞直到你的函数成功或返回错误，包括 hystrix 电路错误
// DoC 相对于 Do 支持ctx
func DoC(ctx context.Context, name string, run runFuncC, fallback fallbackFuncC) error {
	// done 是否正常结束
	done := make(chan struct{}, 1)

	// 需求命令
	r := func(ctx context.Context) error {
		err := run(ctx)
		if err != nil {
			return err
		}

		done <- struct{}{}
		return nil
	}

	// 回退命令
	f := func(ctx context.Context, e error) error {
		err := fallback(ctx, e)
		if err != nil {
			return err
		}

		done <- struct{}{}
		return nil
	}

	// 执行
	var errChan chan error
	if fallback == nil {
		errChan = GoC(ctx, name, r, nil)
	} else {
		errChan = GoC(ctx, name, r, f)
	}

	// 查看结果
	select {
	case <-done:
		return nil
	case err := <-errChan:
		return err
	}
}

func (c *command) reportEvent(eventType string) {
	c.Lock()
	defer c.Unlock()

	c.events = append(c.events, eventType)
}

// errorWithFallback triggers the fallback while reporting the appropriate metric events.
// errorWithFallback 在报告适当的度量事件时触发回退。
func (c *command) errorWithFallback(ctx context.Context, err error) {
	eventType := "failure"
	if err == ErrCircuitOpen {
		eventType = "short-circuit"
	} else if err == ErrMaxConcurrency {
		eventType = "rejected"
	} else if err == ErrTimeout {
		eventType = "timeout"
	} else if err == context.Canceled {
		eventType = "context_canceled"
	} else if err == context.DeadlineExceeded {
		eventType = "context_deadline_exceeded"
	}

	c.reportEvent(eventType)
	fallbackErr := c.tryFallback(ctx, err)
	if fallbackErr != nil {
		c.errChan <- fallbackErr
	}
}

func (c *command) tryFallback(ctx context.Context, err error) error {
	if c.fallback == nil {
		// If we don't have a fallback return the original error.
		// 如果我们没有回退，则返回原始错误。
		return err
	}

	fallbackErr := c.fallback(ctx, err)
	if fallbackErr != nil {
		c.reportEvent("fallback-failure")
		return fmt.Errorf("fallback failed with '%v'. run error was '%v'", fallbackErr, err)
	}

	c.reportEvent("fallback-success")

	return nil
}
