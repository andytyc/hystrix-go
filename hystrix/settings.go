package hystrix

import (
	"sync"
	"time"
)

var (
	// DefaultTimeout is how long to wait for command to complete, in milliseconds
	DefaultTimeout = 1000
	// DefaultMaxConcurrent is how many commands of the same type can run at the same time
	DefaultMaxConcurrent = 10
	// DefaultVolumeThreshold is the minimum number of requests needed before a circuit can be tripped due to health
	DefaultVolumeThreshold = 20
	// DefaultSleepWindow is how long, in milliseconds, to wait after a circuit opens before testing for recovery
	DefaultSleepWindow = 5000
	// DefaultErrorPercentThreshold causes circuits to open once the rolling measure of errors exceeds this percent of requests
	DefaultErrorPercentThreshold = 50
	// DefaultLogger is the default logger that will be used in the Hystrix package. By default prints nothing.
	DefaultLogger = NoopLogger{}
)

/*
config := hystrix.CommandConfig{
	Timeout:                2000, //执行command的超时时间
	MaxConcurrentRequests:  8,    //command的最大并发量
	SleepWindow:            1000, //过多长时间，熔断器再次检测是否开启。单位毫秒
	ErrorPercentThreshold:  30,   //错误率 请求数量大于等于RequestVolumeThreshold并且错误率到达这个百分比后就会启动
	RequestVolumeThreshold: 5,    //请求阈值(一个统计窗口10秒内请求数量)  熔断器是否打开首先要满足这个条件；这里的设置表示至少有5个请求才进行ErrorPercentThreshold错误百分比计算
}
*/
type Settings struct {
	// 执行command的超时时间
	Timeout time.Duration
	// command的最大并发量
	MaxConcurrentRequests int
	// 过多长时间，熔断器再次检测是否开启。单位毫秒
	RequestVolumeThreshold uint64
	// 错误率 请求数量大于等于RequestVolumeThreshold并且错误率到达这个百分比后就会启动
	SleepWindow time.Duration
	// 请求阈值(一个统计窗口10秒内请求数量)  熔断器是否打开首先要满足这个条件；这里的设置表示至少有5个请求才进行ErrorPercentThreshold错误百分比计算
	ErrorPercentThreshold int
}

// CommandConfig is used to tune circuit settings at runtime
// CommandConfig 用于在运行时调整电路设置
type CommandConfig struct {
	Timeout                int `json:"timeout"`
	MaxConcurrentRequests  int `json:"max_concurrent_requests"`
	RequestVolumeThreshold int `json:"request_volume_threshold"`
	SleepWindow            int `json:"sleep_window"`
	ErrorPercentThreshold  int `json:"error_percent_threshold"`
}

var circuitSettings map[string]*Settings
var settingsMutex *sync.RWMutex
var log logger

func init() {
	circuitSettings = make(map[string]*Settings)
	settingsMutex = &sync.RWMutex{}
	log = DefaultLogger
}

// Configure applies settings for a set of circuits
// ConfigureCommand 为电路应用设置一组，多个
func Configure(cmds map[string]CommandConfig) {
	for k, v := range cmds {
		ConfigureCommand(k, v)
	}
}

// ConfigureCommand applies settings for a circuit
// ConfigureCommand 为电路应用设置，单个
func ConfigureCommand(name string, config CommandConfig) {
	settingsMutex.Lock()
	defer settingsMutex.Unlock()

	timeout := DefaultTimeout
	if config.Timeout != 0 {
		timeout = config.Timeout
	}

	max := DefaultMaxConcurrent
	if config.MaxConcurrentRequests != 0 {
		max = config.MaxConcurrentRequests
	}

	volume := DefaultVolumeThreshold
	if config.RequestVolumeThreshold != 0 {
		volume = config.RequestVolumeThreshold
	}

	sleep := DefaultSleepWindow
	if config.SleepWindow != 0 {
		sleep = config.SleepWindow
	}

	errorPercent := DefaultErrorPercentThreshold
	if config.ErrorPercentThreshold != 0 {
		errorPercent = config.ErrorPercentThreshold
	}

	circuitSettings[name] = &Settings{
		Timeout:                time.Duration(timeout) * time.Millisecond,
		MaxConcurrentRequests:  max,
		RequestVolumeThreshold: uint64(volume),
		SleepWindow:            time.Duration(sleep) * time.Millisecond,
		ErrorPercentThreshold:  errorPercent,
	}
}

func getSettings(name string) *Settings {
	settingsMutex.RLock()
	s, exists := circuitSettings[name]
	settingsMutex.RUnlock()

	if !exists {
		ConfigureCommand(name, CommandConfig{})
		s = getSettings(name)
	}

	return s
}

func GetCircuitSettings() map[string]*Settings {
	copy := make(map[string]*Settings)

	settingsMutex.RLock()
	for key, val := range circuitSettings {
		copy[key] = val
	}
	settingsMutex.RUnlock()

	return copy
}

// SetLogger configures the logger that will be used. This only applies to the hystrix package.
// SetLogger 配置将使用的记录器。这仅适用于 hystrix 包。
func SetLogger(l logger) {
	log = l
}
