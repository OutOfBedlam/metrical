package httpstat

import (
	"io"
	"net/http"
	"time"

	"github.com/OutOfBedlam/metric"
)

type ServerMeter struct {
	name    string
	ch      chan<- metric.Measurement
	handler http.Handler
}

func NewHandler(ch chan<- metric.Measurement, handler http.Handler) *ServerMeter {
	return &ServerMeter{
		name:    "http",
		ch:      ch,
		handler: handler,
	}
}

func (sm *ServerMeter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tick := time.Now()
	reqCounter := &ByteCounter{r: r.Body}
	r.Body = reqCounter
	rsp := &ResponseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
	defer func() {
		measure := metric.Measurement{Name: "http"}
		measure.AddField(metric.Field{Name: "requests", Value: 1, Type: metric.CounterType(metric.UnitShort)})
		measure.AddField(metric.Field{Name: "latency", Value: float64(time.Since(tick).Nanoseconds()), Type: metric.HistogramType(metric.UnitDuration, 100, 0.5, 0.9, 0.99)})
		measure.AddField(metric.Field{Name: "write_bytes", Value: float64(rsp.responseBytes), Type: metric.MeterType(metric.UnitBytes)})
		measure.AddField(metric.Field{Name: "read_bytes", Value: float64(reqCounter.total), Type: metric.MeterType(metric.UnitBytes)})
		measure.AddField(metric.Field{Name: rsp.StatusCodeCategory(), Value: 1, Type: metric.CounterType(metric.UnitShort)})
		sm.ch <- measure

		if err := recover(); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}()
	sm.handler.ServeHTTP(rsp, r)
}

func (sm *ServerMeter) String() string {
	return "ServerMeter: HTTP server metrics"
}

type ResponseWriterWrapper struct {
	http.ResponseWriter
	responseBytes int
	statusCode    int
}

func (w *ResponseWriterWrapper) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.responseBytes += n
	return n, err
}

func (w *ResponseWriterWrapper) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.statusCode = statusCode
}

func (w *ResponseWriterWrapper) StatusCodeCategory() string {
	switch sc := w.statusCode; {
	case sc >= 100 && sc < 200:
		return "status_1xx"
	case sc >= 200 && sc < 300:
		return "status_2xx"
	case sc >= 300 && sc < 400:
		return "status_3xx"
	case sc >= 400 && sc < 500:
		return "status_4xx"
	case sc >= 500:
		return "status_5xx"
	default:
		return "satus_unknown"
	}
}

type ByteCounter struct {
	r     io.ReadCloser
	total int64
}

func (bc *ByteCounter) Read(p []byte) (int, error) {
	n, err := bc.r.Read(p)
	if n > 0 {
		bc.total += int64(n)
	}
	return n, err
}

func (bc *ByteCounter) Close() error {
	return bc.r.Close()
}
