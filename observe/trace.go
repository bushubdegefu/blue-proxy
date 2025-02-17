package observe

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/bushubdegefu/blue-proxy.com/configs"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var AppTracer = otel.Tracer(fmt.Sprintf("echo-server-%v", configs.AppConfig.Get("APP_NAME")))

func InitTracer() *sdktrace.TracerProvider {
	traceExporter := configs.AppConfig.Get("TRACE_EXPORTER")
	tracerHost := configs.AppConfig.Get("TRACER_HOST")
	tracerPort := configs.AppConfig.GetOrDefault("TRACER_PORT", "9411")

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(configs.AppConfig.Get("APP_NAME")),
		)),
	)
	// app logger with jager

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	// otel.SetErrorHandler(&otelErrorHandler{logger: app_logger})

	const traceExporterFiber = "fiber"

	if (traceExporter != "" && tracerHost != "") || traceExporter == traceExporterFiber {
		var (
			exporter sdktrace.SpanExporter
			// err      error
		)

		switch strings.ToLower(traceExporter) {
		case "jaeger":
			// app_logger.Log("Exporting traces to jaeger.")

			exporter, _ = otlptracegrpc.New(context.Background(), otlptracegrpc.WithInsecure(),
				otlptracegrpc.WithEndpoint(fmt.Sprintf("%s:%s", tracerHost, tracerPort)))

			batcher := sdktrace.NewBatchSpanProcessor(exporter)
			tp.RegisterSpanProcessor(batcher)
		}
	}
	return tp
}

func EchoAppSpanner(ctx echo.Context, span_name string) (context.Context, oteltrace.Span) {
	gen, _ := uuid.NewV7()
	id := gen.String()

	//  getting request body
	trace, span := AppTracer.Start(ctx.Request().Context(), span_name,
		oteltrace.WithAttributes(attribute.String("id", id)),
		// oteltrace.WithAttributes(attribute.String("request", string(ctx.Request().RequestURI))),
	)
	return trace, span
}

type RouteTracer struct {
	Tracer context.Context
	Span   oteltrace.Span
}
