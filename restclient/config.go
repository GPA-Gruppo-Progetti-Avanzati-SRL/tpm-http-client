package restclient

import (
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/hartracing"
	"github.com/opentracing/opentracing-go"
	"time"
)

const (
	RequestTraceNameOpNamePlaceHolder    = "{op-name}"
	RequestTraceNameRequestIdPlaceHolder = "{req-id}"
	RequestIdTraceTag                    = "req-id"
	OpNameTraceTag                       = "op-name"
	LraHttpContextTraceTag               = "long-running-action"
)

type Header struct {
	Name  string `mapstructure:"name" json:"name" yaml:"name"`
	Value string `mapstructure:"value" json:"value" yaml:"value"`
}

type Config struct {
	RestTimeout       time.Duration    `mapstructure:"timeout,omitempty" json:"timeout,omitempty" yaml:"timeout,omitempty"`
	SkipVerify        bool             `mapstructure:"skv,omitempty" json:"skv,omitempty" yaml:"skv,omitempty"`
	Headers           []Header         `mapstructure:"headers,omitempty" json:"headers,omitempty" yaml:"headers,omitempty"`
	TraceGroupName    string           `mapstructure:"trace-group-name,omitempty" json:"trace-group-name,omitempty" yaml:"trace-group-name,omitempty"`
	TraceRequestName  string           `mapstructure:"trace-req-name,omitempty" json:"trace-req-name,omitempty" yaml:"trace-req-name,omitempty"`
	RetryCount        int              `mapstructure:"retry-count,omitempty" json:"retry-count,omitempty" yaml:"retry-count,omitempty"`
	RetryWaitTime     time.Duration    `mapstructure:"retry-wait-time,omitempty" json:"retry-wait-time,omitempty" yaml:"retry-wait-time,omitempty"`
	RetryMaxWaitTime  time.Duration    `mapstructure:"retry-max-wait-time,omitempty" json:"retry-max-wait-time,omitempty" yaml:"retry-max-wait-time,omitempty"`
	RetryOnHttpError  []int            `mapstructure:"retry-on-errors,omitempty" json:"retry-on-errors,omitempty" yaml:"retry-on-errors,omitempty"`
	HarTracingEnabled bool             `mapstructure:"har-tracing-enabled,omitempty" json:"har-tracing-enabled,omitempty" yaml:"har-tracing-enabled,omitempty"`
	Span              opentracing.Span `mapstructure:"-" json:"-" yaml:"-"`
	HarSpan           hartracing.Span  `mapstructure:"-" json:"-" yaml:"-"`
}

func (cfg *Config) IsHarTracingEnabled() bool {
	return cfg.HarTracingEnabled && !hartracing.GlobalTracer().IsNil()
}

type Option func(o *Config)

func WithSpan(span opentracing.Span) Option {
	return func(o *Config) {
		o.Span = span
	}
}

func WithHarSpan(span hartracing.Span) Option {
	return func(o *Config) {
		o.HarSpan = span
	}
}

func WithHarTracingEnabled(b bool) Option {
	return func(o *Config) {
		o.HarTracingEnabled = b
	}
}

func WithTraceGroupName(opn string) Option {
	return func(o *Config) {
		o.TraceGroupName = opn
	}
}

func WithTraceRequestName(opn string) Option {
	return func(o *Config) {
		o.TraceRequestName = opn
	}
}

func WithSkipVerify(b bool) Option {
	return func(o *Config) {
		o.SkipVerify = b
	}
}

func WithTimeout(to time.Duration) Option {
	return func(o *Config) {
		o.RestTimeout = to
	}
}
