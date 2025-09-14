package stringbuilder

import (
	"strings"
	"sync"
)

const (
	maxCap = 1024
)

type Pool struct {
	pool sync.Pool
}

var DefaultPool = NewPool() //nolint:gochecknoglobals // it's ok

// Get retrieves a strings.Builder from the pool.
func (p *Pool) Get() *strings.Builder {
	builder, _ := p.pool.Get().(*strings.Builder)
	builder.Reset()

	return builder
}

// Put returns a strings.Builder to the pool.
func (p *Pool) Put(builder *strings.Builder) {
	if builder.Cap() > maxCap {
		return
	}

	p.pool.Put(builder)
}

// GetStringWithCapacity retrieves a builder, pre-allocates capacity, executes the build function, and returns the string.
func (p *Pool) GetStringWithCapacity(estimatedSize int, buildFn func(*strings.Builder)) string {
	builder := p.Get()

	defer p.Put(builder)

	if estimatedSize > 0 {
		builder.Grow(estimatedSize)
	}

	buildFn(builder)

	return builder.String()
}

// ConcatStrings concatenates multiple strings efficiently.
func ConcatStrings(strLen int, strs ...string) string {
	return DefaultPool.GetStringWithCapacity(strLen, func(b *strings.Builder) {
		for _, s := range strs {
			b.WriteString(s)
		}
	})
}

func NewPool() *Pool {
	return &Pool{
		pool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
}
