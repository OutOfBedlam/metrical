package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OutOfBedlam/metrical/collect"
	"github.com/OutOfBedlam/metrical/input/ps"
	"github.com/OutOfBedlam/metrical/input/runtime"
	"github.com/OutOfBedlam/metrical/output/svg"
)

func main() {
	collector := collect.NewCollector(1 * time.Second)
	collector.AddInput(&ps.PSInput{})
	collector.AddInput(&runtime.Stats{})
	collector.Start()
	defer collector.Stop()

	exporter := collect.NewExporter(1*time.Second, []string{
		"metrical:ps:cpu_percent",
		"metrical:ps:mem_percent",
		"metrical:runtime:goroutines",
	})
	exporter.AddOutput(&svg.SVGOutput{DstDir: "./tmp"}, nil)
	exporter.Start()
	defer exporter.Stop()

	// wait signal ^C
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
}
