package files

import (
	"os"
	"testing"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/stretchr/testify/require"
)

func createTestStorage(t *testing.T) *metric.FileStorage {
	t.Helper()
	os.MkdirAll("../../tmp/store", 0755)
	storage := NewFileStorage("../../tmp/store", 100)
	require.NotNil(t, storage)
	return storage
}

func TestTimeseriesStorage(t *testing.T) {
	storage := createTestStorage(t)
	storage.Open()
	defer func() {
		storage.Close()
		time.Sleep(1 * time.Second)
	}()
	ts := metric.NewTimeSeries(time.Second, 3, metric.NewMeter())
	seriesID, err := metric.NewSeriesID("test_measure:test_field", "Test Measure", time.Second, 3)
	require.NoError(t, err)
	ts.SetMeta(metric.SeriesInfo{
		MeasureName: "test_measure",
		MeasureType: metric.CounterType(metric.UnitShort),
		SeriesID:    seriesID,
	})
	ts.SetListener(func(tb metric.TimeBin, meta any) {
		var prd metric.Product
		if ok := metric.ToProduct(&prd, tb, meta); !ok {
			return
		}
		seriesID, err := metric.NewSeriesID("ts_1m", "Test Measure", time.Second, 3)
		require.NoError(t, err)
		err = storage.Store(seriesID, prd, false)
		require.NoError(t, err)
	})
	ts.Add(1.0)
	time.Sleep(1 * time.Second)
	ts.Add(2.0)
	time.Sleep(1 * time.Second)
	/*
	   require.JSONEq(t, `[`+

	   	`{"ts":"2023-10-01 12:04:05","value":{"samples":1,"max":1,"min":1,"first":1,"last":1,"sum":1}},`+
	   	`{"ts":"2023-10-01 12:04:06","value":{"samples":1,"max":2,"min":2,"first":2,"last":2,"sum":2}}`+
	   	`]`, ts.String())

	   loaded, err := storage.Load("test_measure:test_field", "3s")
	   require.NoError(t, err)

	   require.JSONEq(t, `[`+

	   	`{"ts":"2023-10-01 12:04:05","value":{"samples":1,"max":1,"min":1,"first":1,"last":1,"sum":1}},`+
	   	`{"ts":"2023-10-01 12:04:06","value":{"samples":1,"max":2,"min":2,"first":2,"last":2,"sum":2}}`+
	   	`]`, loaded.String())
	*/
}
