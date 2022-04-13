package rolling

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Timing maintains time Durations for each time bucket.
// The Durations are kept in an array to allow for a variety of
// statistics to be calculated from the source data.
//
// Timing 维护每个时间桶的时间Durations，仅保留最近 60 秒内{rolling:滚动}。应用场景：记录执行命令的耗时
// 持续时间保存在一个数组中，以允许从源数据计算各种统计信息。
//
// 解读:
// Buckets map[int64]*timingBucket 时间序列指标的map (一张向量图) -> {时间序列指标, 时间序列指标, ...}
// 时间序列指标: time: [dur, dur, ...], 一个秒级时间戳对应一个时间序列指标, 指标值是一个耗时数组, 表示：在此刻执行多次命令，而每一次执行的耗时进行记录
type Timing struct {
	Buckets map[int64]*timingBucket
	Mutex   *sync.RWMutex

	// 缓存排序的时间桶 | 对最近60s内的时间桶进行了排序，在这里缓存
	CachedSortedDurations []time.Duration
	// 最新一次缓存排序的时间桶时间 | 比如:获取排序时发现时1秒前排序过，则直接使用缓存的排序结果 CachedSortedDurations
	LastCachedTime int64
}

type timingBucket struct {
	Durations []time.Duration
}

// NewTiming creates a RollingTiming struct.
//
// NewTiming 创建一个 RollingTiming 结构。
func NewTiming() *Timing {
	r := &Timing{
		Buckets: make(map[int64]*timingBucket),
		Mutex:   &sync.RWMutex{},
	}
	return r
}

type byDuration []time.Duration

func (c byDuration) Len() int           { return len(c) }
func (c byDuration) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c byDuration) Less(i, j int) bool { return c[i] < c[j] } // 升序

// SortedDurations returns an array of time.Duration sorted from shortest
// to longest that have occurred in the last 60 seconds.
//
// SortedDurations 返回一个 time.Duration 数组，从最近 60 秒内发生的最短到最长排序。
func (r *Timing) SortedDurations() []time.Duration {
	r.Mutex.RLock()
	t := r.LastCachedTime
	r.Mutex.RUnlock()

	if t+time.Duration(1*time.Second).Nanoseconds() > time.Now().UnixNano() {
		// don't recalculate if current cache is still fresh
		// 如果当前缓存仍然是新鲜的，不要重新计算
		return r.CachedSortedDurations
	}

	var durations byDuration
	now := time.Now()

	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	for timestamp, b := range r.Buckets {
		// TODO: configurable rolling window
		if timestamp >= now.Unix()-60 {
			for _, d := range b.Durations {
				durations = append(durations, d)
			}
		}
	}

	sort.Sort(durations)

	r.CachedSortedDurations = durations
	r.LastCachedTime = time.Now().UnixNano()

	return r.CachedSortedDurations
}

// getCurrentBucket 获取当前秒的桶，没有则创建一个新的
func (r *Timing) getCurrentBucket() *timingBucket {
	r.Mutex.RLock()
	now := time.Now()
	bucket, exists := r.Buckets[now.Unix()]
	r.Mutex.RUnlock()

	if !exists {
		r.Mutex.Lock()
		defer r.Mutex.Unlock()

		r.Buckets[now.Unix()] = &timingBucket{}
		bucket = r.Buckets[now.Unix()]
	}

	return bucket
}

// removeOldBuckets 删除60s前的桶 | 目前仅保留最近10s的桶
//
// 注意 调用时都在 Mutex 锁代码块中执行
func (r *Timing) removeOldBuckets() {
	now := time.Now()

	for timestamp := range r.Buckets {
		// TODO: configurable rolling window
		if timestamp <= now.Unix()-60 {
			delete(r.Buckets, timestamp)
		}
	}
}

// Add appends the time.Duration given to the current time bucket.
//
// Add 增量/追加 | 将 time.Duration 附加到当前时间桶。
func (r *Timing) Add(duration time.Duration) {
	b := r.getCurrentBucket()

	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	b.Durations = append(b.Durations, duration)
	r.removeOldBuckets()
}

// Percentile computes the percentile given with a linear interpolation.
//
// Percentile 计算通过线性插值给出的百分位数。| 返回毫秒ms
func (r *Timing) Percentile(p float64) uint32 {
	sortedDurations := r.SortedDurations()
	length := len(sortedDurations)
	if length <= 0 {
		return 0
	}

	pos := r.ordinal(len(sortedDurations), p) - 1
	return uint32(sortedDurations[pos].Nanoseconds() / 1000000)
}

// ordinal 顺序数 | 计算 percentile(如:50%) 在时间桶[](即:sortedDurations)中的最接近的索引数
//
// percentile 百分位数
func (r *Timing) ordinal(length int, percentile float64) int64 {
	if percentile == 0 && length > 0 {
		return 1
	}

	// Ceil returns the least integer value greater than or equal to x.
	// Ceil 返回大于或等于 x 的最小整数值。
	//
	// Special cases are:
	// 特殊情况是：
	//	Ceil(±0) = ±0
	//	Ceil(±Inf) = ±Inf
	//	Ceil(NaN) = NaN

	return int64(math.Ceil((percentile / float64(100)) * float64(length)))
}

// Mean computes the average timing in the last 60 seconds.
//
// Mean 查询平均值 | 计算过去 60 秒的平均时间。 | 返回的是毫秒ms
func (r *Timing) Mean() uint32 {
	sortedDurations := r.SortedDurations()
	var sum time.Duration
	for _, d := range sortedDurations {
		sum += d
	}

	length := int64(len(sortedDurations))
	if length == 0 {
		return 0
	}

	return uint32(sum.Nanoseconds()/length) / 1000000
}
