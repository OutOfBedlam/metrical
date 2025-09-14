package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OutOfBedlam/metric"
	_ "github.com/mattn/go-sqlite3"
)

func NewStorage(path string, bufferSize int) (metric.Storage, error) {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	ret := &Storage{
		path:   path,
		wChan:  make(chan *Record, bufferSize),
		tables: make(map[string]TableInfo),
	}
	return ret, nil
}

var _ metric.Storage = (*Storage)(nil)

type Storage struct {
	path   string
	wChan  chan *Record
	db     *sql.DB
	tables map[string]TableInfo
}

func (s *Storage) Open() error {
	db, err := sql.Open("sqlite3", s.path)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return err
	}
	s.db = db
	go s.runWriteLoop()
	return nil
}

func (s *Storage) Close() error {
	close(s.wChan)
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return err
		}
		s.db = nil
	}
	return nil
}

func (s *Storage) runWriteLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// periodic shrink of tables
			for _, tableInfo := range s.tables {
				s.shrink(tableInfo)
			}
		case rec := <-s.wChan:
			if rec == nil {
				return
			}
			s.write(rec)
		}
	}
}

type Record struct {
	id      metric.SeriesID
	pd      metric.Product
	closing bool
}

type TableInfo struct {
	name            string
	retentionPeriod time.Duration
}

func TableName(id metric.SeriesID) string {
	return fmt.Sprintf("METRIC_%s", id.ID())
}

func (s *Storage) Store(id metric.SeriesID, pd metric.Product, closing bool) error {
	s.wChan <- &Record{id: id, pd: pd, closing: closing}
	return nil
}

func (s *Storage) Load(id metric.SeriesID, metricName string) ([]metric.Product, error) {
	tableName := TableName(id)
	sqlText := strings.Join([]string{
		"SELECT",
		"name, timestamp, type, samples, value, sum, first_value, last_value, min, max, other",
		"FROM", tableName,
		"WHERE name = ?",
		"ORDER BY timestamp ASC",
	}, " ")
	rows, err := s.db.Query(sqlText, metricName)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			// no data yet
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var results []metric.Product
	for rows.Next() {
		var (
			name       string
			timestamp  time.Time
			mType      string
			samples    sql.NullInt64
			value      sql.NullFloat64
			sum        sql.NullFloat64
			firstValue sql.NullFloat64
			lastValue  sql.NullFloat64
			min        sql.NullFloat64
			max        sql.NullFloat64
			other      sql.NullString
		)
		if err := rows.Scan(&name, &timestamp, &mType, &samples, &value, &sum, &firstValue, &lastValue, &min, &max, &other); err != nil {
			slog.Error("Failed to scan row", "error", err)
			continue
		}
		var pd metric.Product
		pd.Name = name
		pd.Time = timestamp
		pd.Type = mType
		switch mType {
		case "counter":
			pd.Value = &metric.CounterValue{}
			if samples.Valid {
				pd.Value.(*metric.CounterValue).Samples = samples.Int64
			}
			if value.Valid {
				pd.Value.(*metric.CounterValue).Value = value.Float64
			}
		case "gauge":
			pd.Value = &metric.GaugeValue{}
			if samples.Valid {
				pd.Value.(*metric.GaugeValue).Samples = samples.Int64
			}
			if value.Valid {
				pd.Value.(*metric.GaugeValue).Value = value.Float64
			}
			if sum.Valid {
				pd.Value.(*metric.GaugeValue).Sum = sum.Float64
			}
		case "meter":
			pd.Value = &metric.MeterValue{}
			if samples.Valid {
				pd.Value.(*metric.MeterValue).Samples = samples.Int64
			}
			if sum.Valid {
				pd.Value.(*metric.MeterValue).Sum = sum.Float64
			}
			if firstValue.Valid {
				pd.Value.(*metric.MeterValue).First = firstValue.Float64
			}
			if lastValue.Valid {
				pd.Value.(*metric.MeterValue).Last = lastValue.Float64
			}
			if min.Valid {
				pd.Value.(*metric.MeterValue).Min = min.Float64
			}
			if max.Valid {
				pd.Value.(*metric.MeterValue).Max = max.Float64
			}
		case "odometer":
			pd.Value = &metric.OdometerValue{}
			if samples.Valid {
				pd.Value.(*metric.OdometerValue).Samples = samples.Int64
			}
			if firstValue.Valid {
				pd.Value.(*metric.OdometerValue).First = firstValue.Float64
			}
			if lastValue.Valid {
				pd.Value.(*metric.OdometerValue).Last = lastValue.Float64
			}
		case "histogram":
			pd.Value = &metric.HistogramValue{}
			if samples.Valid {
				pd.Value.(*metric.HistogramValue).Samples = samples.Int64
			}
			if other.Valid && other.String != "" {
				var otherMap map[string]float64
				if err := json.Unmarshal([]byte(other.String), &otherMap); err == nil {
					for k, v := range otherMap {
						var p float64
						if _, err := fmt.Sscanf(k, "%f", &p); err == nil {
							pd.Value.(*metric.HistogramValue).P = append(pd.Value.(*metric.HistogramValue).P, p)
							pd.Value.(*metric.HistogramValue).Values = append(pd.Value.(*metric.HistogramValue).Values, v)
						}
					}
				}
			}
		default:
			slog.Warn("Unknown metric type", "type", mType)
			continue
		}
		results = append(results, pd)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return results, nil
	}
	return nil, nil
}

func (s *Storage) write(rec *Record) {
	var tableName string
	tableInfo, exists := s.tables[rec.id.ID()]
	if exists {
		tableName = tableInfo.name
	} else {
		tableName = TableName(rec.id)
		sqlText := strings.Join([]string{
			"CREATE TABLE IF NOT EXISTS",
			tableName,
			"(",
			"name TEXT NOT NULL,",
			"timestamp timestamp NOT NULL,",
			"type TEXT,",
			"samples INTEGER,",
			"value REAL,",
			"sum REAL,",
			"first_value REAL,",
			"last_value REAL,",
			"min REAL,",
			"max REAL,",
			"other TEXT,",
			"PRIMARY KEY (name, timestamp)",
			")",
		}, " ")
		_, err := s.db.Exec(sqlText)
		if err != nil {
			slog.Error("Failed to create table", "table", tableName, "error", err)
			return
		}
		s.tables[rec.id.ID()] = TableInfo{
			name:            tableName,
			retentionPeriod: rec.id.Period() * time.Duration(rec.id.MaxCount()+1),
		}
	}
	columns := []string{
		"name",
		"timestamp",
		"type",
	}
	values := []any{rec.pd.Name, rec.pd.Time, rec.pd.Type}
	switch p := rec.pd.Value.(type) {
	case *metric.CounterValue:
		columns = append(columns, "samples", "value")
		values = append(values, p.Samples, p.Value)
	case *metric.GaugeValue:
		columns = append(columns, "samples", "value", "sum")
		values = append(values, p.Samples, p.Value, p.Sum)
	case *metric.MeterValue:
		columns = append(columns, "samples", "value", "sum", "first_value", "last_value", "min", "max")
		value := 0.0
		if p.Samples > 0 {
			value = p.Sum / float64(p.Samples)
		}
		values = append(values, p.Samples, value, p.Sum, p.First, p.Last, p.Min, p.Max)
	case *metric.OdometerValue:
		columns = append(columns, "samples", "first_value", "last_value")
		values = append(values, p.Samples, p.First, p.Last)
	case *metric.HistogramValue:
		columns = append(columns, "samples", "value")
		value := 0.0
		for i, x := range p.P {
			if i == 0 || x == 0.5 {
				// value is median (P50)
				// or the first percentile if no P50 exists
				value = p.Values[i]
			}
		}
		values = append(values, p.Samples, value)
		// Store percentiles in "other" column as JSON
		other := make(map[string]float64)
		for i, x := range p.P {
			k := fmt.Sprintf("%g", x)
			other[k] = p.Values[i]
		}
		if len(other) > 0 {
			b, _ := json.Marshal(other)
			columns = append(columns, "other")
			values = append(values, string(b))
		}
	}
	sqlText := strings.Join([]string{
		"INSERT OR REPLACE INTO",
		tableName, "(",
		strings.Join(columns, ","),
		")",
		"VALUES (?", strings.Repeat(",?", len(columns)-1), ")",
	}, " ")
	result, err := s.db.Exec(sqlText, values...)
	if err != nil {
		slog.Error("Failed to insert record", "table", tableName, "error", err)
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		slog.Error("Failed to get rows affected", "table", tableName, "error", err)
		return
	}
	if rows == 0 {
		slog.Warn("No rows affected when inserting record", "table", tableName)
	}
}

func (s *Storage) shrink(tableInfo TableInfo) {
	sqlText := fmt.Sprintf("delete from %s where timestamp < ?", tableInfo.name)
	cutoff := time.Now().Add(-tableInfo.retentionPeriod)
	result, err := s.db.Exec(sqlText, cutoff)
	if err != nil {
		slog.Error("Failed to shrink table", "table", tableInfo.name, "error", err)
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		slog.Error("Failed to get rows affected when shrinking table", "table", tableInfo.name, "error", err)
		return
	}
	if rows > 0 {
		slog.Debug("Shrunk table", "table", tableInfo.name, "rows_deleted", rows, "cutoff", cutoff)
	}
}
