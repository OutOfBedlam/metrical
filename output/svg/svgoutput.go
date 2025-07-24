package svg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/collect"
)

type SVGOutput struct {
	DstDir string
}

func (s *SVGOutput) Export(req collect.ExportReq) error {
	var metricName string = req.Name
	var ss *metric.TimeSeriesSnapshot[float64] = req.Data

	lastValue := ss.Values[len(ss.Values)-1]
	dstFile := filepath.Join(s.DstDir, fmt.Sprintf("%s.svg", strings.ReplaceAll(metricName, ":", "_")))
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	svg := NewCanvas(200, 80)
	svg.Title = fmt.Sprintf("%s - %.f%s", req.Title, lastValue, req.Unit)
	svg.StrokeWidth = 1.5
	svg.GridYMin = 0
	svg.GridYMax = 100
	svg.GridMaxCount = ss.MaxCount
	if err := svg.Export(out, ss.Times, ss.Values); err != nil {
		panic(fmt.Errorf("failed to generate SVG: %v", err))
	}
	out.Close()
	return nil
}
