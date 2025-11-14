package httpstat

import (
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/OutOfBedlam/metric"
)

type ServerMeter struct {
	name    string
	ch      chan<- *metric.Gather
	handler http.Handler
}

func NewHandler(ch chan<- *metric.Gather, handler http.Handler) *ServerMeter {
	return &ServerMeter{
		name:    "http",
		ch:      ch,
		handler: handler,
	}
}

var counterType = metric.CounterType(metric.UnitShort)
var bytesCounterType = metric.CounterType(metric.UnitBytes)
var histogramType = metric.HistogramType(metric.UnitDuration)

func (sm *ServerMeter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tick := time.Now()
	reqCounter := &ByteCounter{r: r.Body}
	r.Body = reqCounter
	rsp := &ResponseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
	defer func() {
		measure := &metric.Gather{}
		measure.Add("http:requests", 1, counterType)
		measure.Add("http:latency", float64(time.Since(tick).Nanoseconds()), histogramType)
		measure.Add("http:bytes_sent", float64(rsp.responseBytes), bytesCounterType)
		measure.Add("http:bytes_recv", float64(reqCounter.total), bytesCounterType)
		measure.Add(fmt.Sprintf("http:status_%dxx", rsp.statusCode/100), 1, counterType)
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
	headerWritten bool
	responseBytes int
	statusCode    int
}

var _ http.ResponseWriter = (*ResponseWriterWrapper)(nil)
var _ http.Flusher = (*ResponseWriterWrapper)(nil)

func (w *ResponseWriterWrapper) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.responseBytes += n
	return n, err
}

func (w *ResponseWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *ResponseWriterWrapper) WriteHeader(statusCode int) {
	if w.headerWritten {
		// http: superfluous response.WriteHeader call
		debug.PrintStack()
		return
	}
	w.headerWritten = true
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
