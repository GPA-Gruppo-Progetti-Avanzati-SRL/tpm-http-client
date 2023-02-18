package restclient

import (
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-http-archive/hartracing"
	"github.com/opentracing/opentracing-go"
)

type ExecutionContext struct {
	OpName    string           `yaml:"op-name,omitempty" mapstructure:"op-name,omitempty" json:"op-name,omitempty"`
	RequestId string           `yaml:"request-id,omitempty" mapstructure:"request-id,omitempty" json:"request-id,omitempty"`
	LRAId     string           `yaml:"lra-id,omitempty" mapstructure:"lra-id,omitempty" json:"lra-id,omitempty"`
	Span      opentracing.Span `yaml:"-" mapstructure:"-" json:"-"`
	HarSpan   hartracing.Span  `yaml:"-" mapstructure:"-" json:"-"`
}

type ExecutionContextOption func(*ExecutionContext)

func ExecutionWithRequestId(reqId string) ExecutionContextOption {
	return func(ctx *ExecutionContext) {
		ctx.RequestId = reqId
	}
}

func ExecutionWithOpName(opName string) ExecutionContextOption {
	return func(ctx *ExecutionContext) {
		ctx.OpName = opName
	}
}

func ExecutionWithLraId(lraId string) ExecutionContextOption {
	return func(ctx *ExecutionContext) {
		ctx.LRAId = lraId
	}
}

func ExecutionWithSpan(span opentracing.Span) ExecutionContextOption {
	return func(ctx *ExecutionContext) {
		ctx.Span = span
	}
}

func ExecutionWithHarSpan(span hartracing.Span) ExecutionContextOption {
	return func(ctx *ExecutionContext) {
		ctx.HarSpan = span
	}
}
