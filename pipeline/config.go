package pipeline

import (
	"runtime"
	"time"
)

// ErrorPolicy controls what a stage runner does when a stage returns an error
// (or panics, which is converted into an error) while processing a batch.
type ErrorPolicy int

const (
	// SkipAndCount drops the offending batch's output, increments the stage
	// error counter, and keeps the pipeline running. This is the default and
	// is appropriate for streaming analytics where one bad event must not take
	// down the whole stream.
	SkipAndCount ErrorPolicy = iota
	// FailFast cancels the pipeline context on the first error so every
	// goroutine drains and exits. The first error is returned from the run.
	FailFast
)

// CollectPolicy controls how a Collect sink behaves when the caller-provided
// bounded sink cannot immediately accept an element.
type CollectPolicy int

const (
	// Block applies back-pressure: the drain blocks until the sink can accept
	// the element (or the context is cancelled). This propagates back-pressure
	// all the way upstream through the bounded channels.
	Block CollectPolicy = iota
	// DropOnFull drops the element and counts it instead of blocking. Use this
	// when freshness matters more than completeness.
	DropOnFull
)

// Config holds the tunable runtime hyper-parameters for a pipeline. Construct
// it implicitly via New(opts...) and the With* functional options.
type Config struct {
	// BatchSize is the maximum number of elements accumulated before a batch is
	// handed to the first stage. Larger batches amortise channel/scheduling
	// overhead at the cost of latency and per-batch memory.
	BatchSize int
	// WorkerCount is the number of goroutines fanned out per stage that draw
	// from the stage's inbound channel concurrently.
	WorkerCount int
	// ChannelBufferDepth is the buffer size (in batches) of each inter-stage
	// channel. 0 means unbuffered (fully synchronous hand-off).
	ChannelBufferDepth int
	// BatchTimeout flushes a partial batch after this duration so low-volume
	// streams do not stall waiting to fill a batch. <= 0 disables the timer.
	BatchTimeout time.Duration
	// OnError selects the error-handling policy. Defaults to SkipAndCount.
	OnError ErrorPolicy
	// MetricsSink, if set, receives the per-stage metrics snapshot at the end
	// of every run. This is the seam an API caller uses to forward metrics to
	// an analytics store (e.g. ClickHouse).
	MetricsSink MetricsSink
}

// Option mutates a Config. Options are applied in order over the defaults.
type Option func(*Config)

func defaultConfig() Config {
	return Config{
		BatchSize:          256,
		WorkerCount:        runtime.NumCPU(),
		ChannelBufferDepth: 8,
		BatchTimeout:       5 * time.Millisecond,
		OnError:            SkipAndCount,
	}
}

// normalize clamps invalid values to safe minimums so degenerate configs
// (e.g. WorkerCount=0) still run correctly.
func (c *Config) normalize() {
	if c.BatchSize < 1 {
		c.BatchSize = 1
	}
	if c.WorkerCount < 1 {
		c.WorkerCount = 1
	}
	if c.ChannelBufferDepth < 0 {
		c.ChannelBufferDepth = 0
	}
}

// WithBatchSize sets the maximum dynamic batch size.
func WithBatchSize(n int) Option { return func(c *Config) { c.BatchSize = n } }

// WithWorkerCount sets the per-stage fan-out worker count.
func WithWorkerCount(n int) Option { return func(c *Config) { c.WorkerCount = n } }

// WithChannelBufferDepth sets the inter-stage channel buffer depth (in batches).
func WithChannelBufferDepth(n int) Option { return func(c *Config) { c.ChannelBufferDepth = n } }

// WithBatchTimeout sets the partial-batch flush timeout. <= 0 disables it.
func WithBatchTimeout(d time.Duration) Option { return func(c *Config) { c.BatchTimeout = d } }

// WithErrorPolicy selects SkipAndCount (default) or FailFast.
func WithErrorPolicy(p ErrorPolicy) Option { return func(c *Config) { c.OnError = p } }

// WithMetricsSink sets the sink that receives the metrics snapshot per run.
func WithMetricsSink(s MetricsSink) Option { return func(c *Config) { c.MetricsSink = s } }
