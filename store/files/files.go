package files

import (
	"github.com/OutOfBedlam/metric"
)

func NewFileStorage(dir string, bufferSize int) *metric.FileStorage {
	return metric.NewFileStorage(dir, bufferSize)
}
