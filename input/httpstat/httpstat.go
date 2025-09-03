package httpstat

import (
	"fmt"
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

var counterType = metric.CounterType(metric.UnitShort)
var bytesMeterType = metric.MeterType(metric.UnitBytes)
var histogramType = metric.HistogramType(metric.UnitDuration)

func (sm *ServerMeter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tick := time.Now()
	reqCounter := &ByteCounter{r: r.Body}
	r.Body = reqCounter
	rsp := &ResponseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
	defer func() {
		measure := metric.Measurement{Name: "http"}
		measure.AddField(metric.Field{Name: "requests", Value: 1, Type: counterType})
		measure.AddField(metric.Field{Name: "latency", Value: float64(time.Since(tick).Nanoseconds()), Type: histogramType})
		measure.AddField(metric.Field{Name: "write_bytes", Value: float64(rsp.responseBytes), Type: bytesMeterType})
		measure.AddField(metric.Field{Name: "read_bytes", Value: float64(reqCounter.total), Type: bytesMeterType})
		measure.AddField(metric.Field{Name: fmt.Sprintf("status_%dxx", rsp.statusCode/100), Value: 1, Type: counterType})
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
