package rolling

import (
	"sync"
	"time"
)

// Number tracks a numberBucket over a bounded number of
// time buckets. Currently the buckets are one second long and only the last 10 seconds are kept.
//
// Number 在一定数量的时间桶上跟踪 numberBucket。
// 目前，存储桶的长度为一秒，仅保留最后 10 秒{rolling:滚动}。
type Number struct {
	Buckets map[int64]*numberBucket
	Mutex   *sync.RWMutex
}

type numberBucket struct {
	Value float64
}

// NewNumber initializes a RollingNumber struct.
//
// NewNumber 初始化一个 RollingNumber 结构。
func NewNumber() *Number {
	r := &Number{
		Buckets: make(map[int64]*numberBucket),
		Mutex:   &sync.RWMutex{},
	}
	return r
}

// getCurrentBucket 获取当前秒的桶，没有则创建一个新的
//
// 注意 调用时都在 Mutex 锁代码块中执行
func (r *Number) getCurrentBucket() *numberBucket {
	now := time.Now().Unix()
	var bucket *numberBucket
	var ok bool

	if bucket, ok = r.Buckets[now]; !ok {
		bucket = &numberBucket{}
		r.Buckets[now] = bucket
	}

	return bucket
}

// removeOldBuckets 删除10s前的桶 | 目前仅保留最近10s的桶
//
// 注意 调用时都在 Mutex 锁代码块中执行
func (r *Number) removeOldBuckets() {
	now := time.Now().Unix() - 10

	for timestamp := range r.Buckets {
		// TODO: configurable rolling window
		// TODO: 可配置的滚动窗口
		if timestamp <= now {
			delete(r.Buckets, timestamp)
		}
	}
}

// Increment increments the number in current timeBucket.
//
// Increment 增量统计 | 增加当前 timeBucket 中的数字。
func (r *Number) Increment(i float64) {
	if i == 0 {
		return
	}

	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	b := r.getCurrentBucket()
	b.Value += i
	r.removeOldBuckets()
}

// UpdateMax updates the maximum value in the current bucket.
//
// UpdateMax 更新峰值 | 更新当前桶中的最大值。| 即：统计在当前秒内最大的值b.Value 小于传入的 n, 则更新最大值为n
func (r *Number) UpdateMax(n float64) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	b := r.getCurrentBucket()
	if n > b.Value {
		b.Value = n
	}
	r.removeOldBuckets()
}

// Sum sums the values over the buckets in the last 10 seconds.
//
// Sum 查询总和 | 时间段内的指标总和 | 对过去 10 秒内桶中的值求和。
func (r *Number) Sum(now time.Time) float64 {
	sum := float64(0)

	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	for timestamp, bucket := range r.Buckets {
		// TODO: configurable rolling window
		// TODO: 可配置的滚动窗口
		if timestamp >= now.Unix()-10 {
			sum += bucket.Value
		}
	}

	return sum
}

// Max returns the maximum value seen in the last 10 seconds.
//
// Max 查询峰值 | 时间段内的指标最大值 | 返回过去 10 秒内看到的最大值。
func (r *Number) Max(now time.Time) float64 {
	var max float64

	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	for timestamp, bucket := range r.Buckets {
		// TODO: configurable rolling window
		// TODO: 可配置的滚动窗口
		if timestamp >= now.Unix()-10 {
			if bucket.Value > max {
				max = bucket.Value
			}
		}
	}

	return max
}

// Avg 查询平均值 | 时间段内的指标平均值 | 返回过去 10 秒内看到的平均值。
func (r *Number) Avg(now time.Time) float64 {
	return r.Sum(now) / 10
}
