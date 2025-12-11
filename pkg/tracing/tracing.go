package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Span struct {
	span   trace.Span
	logger *zap.Logger
	ctx    context.Context
}

func StartSpan(ctx context.Context, tracer trace.Tracer, logger *zap.Logger, name string, attrs ...attribute.KeyValue) (context.Context, *Span) {
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))

	return ctx, &Span{
		span:   span,
		logger: logger,
		ctx:    ctx,
	}
}

func (s *Span) End(err error) {
	if err != nil {
		s.span.SetStatus(codes.Error, err.Error())
		s.span.RecordError(err)
	} else {
		s.span.SetStatus(codes.Ok, "")
	}

	s.span.End()
}

func (s *Span) AddEvent(name string, attrs ...attribute.KeyValue) {
	s.span.AddEvent(name, trace.WithAttributes(attrs...))
}

func (s *Span) SetAttributes(attrs ...attribute.KeyValue) {
	s.span.SetAttributes(attrs...)
}
