// A helper pakc

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	sqlcgen "go-conductor/db-internal/sqlc-generated"

	_ "modernc.org/sqlite"
)

const accountNumber = "204772699175"

type cloudWatchAlarmEvent struct {
	Version    string   `json:"version"`
	ID         string   `json:"id"`
	DetailType string   `json:"detail-type"`
	Source     string   `json:"source"`
	Account    string   `json:"account"`
	Time       string   `json:"time"`
	Region     string   `json:"region"`
	Resources  []string `json:"resources"`
	Detail     struct {
		AlarmName string `json:"alarmName"`
		State     struct {
			Value      string `json:"value"`
			Reason     string `json:"reason"`
			ReasonData string `json:"reasonData"`
			Timestamp  string `json:"timestamp"`
		} `json:"state"`
		PreviousState struct {
			Value      string `json:"value"`
			Reason     string `json:"reason"`
			ReasonData string `json:"reasonData"`
			Timestamp  string `json:"timestamp"`
		} `json:"previousState"`
		Configuration struct {
			Metrics []struct {
				ID         string `json:"id"`
				MetricStat struct {
					Metric struct {
						Namespace  string            `json:"namespace"`
						Name       string            `json:"name"`
						Dimensions map[string]string `json:"dimensions"`
					} `json:"metric"`
					Period int    `json:"period"`
					Stat   string `json:"stat"`
				} `json:"metricStat"`
				ReturnData bool `json:"returnData"`
			} `json:"metrics"`
			Description string `json:"description"`
		} `json:"configuration"`
	} `json:"detail"`
}

type reasonData struct {
	Version             string               `json:"version"`
	QueryDate           string               `json:"queryDate"`
	StartDate           string               `json:"startDate,omitempty"`
	Statistic           string               `json:"statistic"`
	Period              int                  `json:"period"`
	RecentDatapoints    []float64            `json:"recentDatapoints"`
	Threshold           float64              `json:"threshold"`
	EvaluatedDatapoints []evaluatedDatapoint `json:"evaluatedDatapoints"`
}

type evaluatedDatapoint struct {
	Timestamp   string   `json:"timestamp"`
	SampleCount *float64 `json:"sampleCount,omitempty"`
	Value       *float64 `json:"value,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	paths, err := testPaths()
	if err != nil {
		return err
	}
	if len(os.Args) > 2 {
		return fmt.Errorf("usage: go run ./DB_TESTDATA/scripts [target-db-path]")
	}
	if len(os.Args) == 2 {
		paths.dbPath = os.Args[1]
	}
	if paths.dbPath == "" {
		return fmt.Errorf("target db path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(paths.dbPath), 0o755); err != nil {
		return fmt.Errorf("create target db directory: %w", err)
	}
	if err := removeSQLiteFiles(paths.dbPath); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", paths.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	schemaSQL, err := os.ReadFile(paths.schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(schemaSQL)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM sqs_alarm_messages"); err != nil {
		return fmt.Errorf("clear alarm messages: %w", err)
	}

	queries := sqlcgen.New(db)
	times := chainedTimes()

	for _, messageTime := range times {
		body, err := alarmMessageBody(messageTime)
		if err != nil {
			return err
		}

		_, err = queries.InsertSQSAlarmMessageAt(ctx, sqlcgen.InsertSQSAlarmMessageAtParams{
			ReceivedAt:       sqliteTime(messageTime),
			RawMessageBody:   body,
			AwsAccountNumber: accountNumber,
		})
		if err != nil {
			return fmt.Errorf("insert alarm at %s: %w", messageTime.Format(time.RFC3339), err)
		}
	}

	fmt.Printf("inserted %d chained alarm messages into %s\n", len(times), paths.dbPath)
	return nil
}

func chainedTimes() []time.Time {
	start := time.Date(2026, 5, 12, 20, 0, 0, 0, time.UTC)
	offsets := []time.Duration{
		0,
		12 * time.Minute,
		24 * time.Minute,
		36 * time.Minute,
		48 * time.Minute,
		59 * time.Minute,
		71 * time.Minute,
		83 * time.Minute,
	}

	times := make([]time.Time, 0, len(offsets))
	for _, offset := range offsets {
		times = append(times, start.Add(offset))
	}
	return times
}

func alarmMessageBody(eventTime time.Time) (string, error) {
	datapointTime := eventTime.Truncate(time.Second)
	previousTime := eventTime.Add(-4 * time.Minute)
	previousDatapointTime := previousTime.Truncate(time.Second)
	value := 99.83333333333333

	currentReasonData, err := json.Marshal(reasonData{
		Version:          "1.0",
		QueryDate:        cloudWatchTime(eventTime),
		StartDate:        cloudWatchTime(datapointTime),
		Statistic:        "Average",
		Period:           20,
		RecentDatapoints: []float64{value},
		Threshold:        20.0,
		EvaluatedDatapoints: []evaluatedDatapoint{
			{
				Timestamp:   cloudWatchTime(datapointTime),
				SampleCount: ptr(1.0),
				Value:       ptr(value),
			},
		},
	})
	if err != nil {
		return "", err
	}

	previousReasonData, err := json.Marshal(reasonData{
		Version:          "1.0",
		QueryDate:        cloudWatchTime(previousTime),
		Statistic:        "Average",
		Period:           20,
		RecentDatapoints: []float64{},
		Threshold:        20.0,
		EvaluatedDatapoints: []evaluatedDatapoint{
			{
				Timestamp: cloudWatchTime(previousDatapointTime),
			},
		},
	})
	if err != nil {
		return "", err
	}

	event := cloudWatchAlarmEvent{
		Version:    "0",
		ID:         fmt.Sprintf("test-alarm-%s", eventTime.Format("20060102T150405")),
		DetailType: "CloudWatch Alarm State Change",
		Source:     "aws.cloudwatch",
		Account:    accountNumber,
		Time:       eventTime.Format(time.RFC3339),
		Region:     "us-west-2",
		Resources: []string{
			"arn:aws:cloudwatch:us-west-2:204772699175:alarm:debian-cpu-spin-high-cpu",
		},
	}
	event.Detail.AlarmName = "debian-cpu-spin-high-cpu"
	event.Detail.State.Value = "ALARM"
	event.Detail.State.Reason = fmt.Sprintf("Threshold Crossed: 1 datapoint [99.83333333333333 (%s)] was greater than the threshold (20.0).", datapointTime.Format("02/01/06 15:04:05"))
	event.Detail.State.ReasonData = string(currentReasonData)
	event.Detail.State.Timestamp = cloudWatchTime(eventTime)
	event.Detail.PreviousState.Value = "INSUFFICIENT_DATA"
	event.Detail.PreviousState.Reason = "Insufficient Data: 1 datapoint was unknown."
	event.Detail.PreviousState.ReasonData = string(previousReasonData)
	event.Detail.PreviousState.Timestamp = cloudWatchTime(previousTime)
	event.Detail.Configuration.Description = "Triggers when debian-cpu-spin CPU average is above 20 percent for one 20-second period"
	event.Detail.Configuration.Metrics = append(event.Detail.Configuration.Metrics, struct {
		ID         string `json:"id"`
		MetricStat struct {
			Metric struct {
				Namespace  string            `json:"namespace"`
				Name       string            `json:"name"`
				Dimensions map[string]string `json:"dimensions"`
			} `json:"metric"`
			Period int    `json:"period"`
			Stat   string `json:"stat"`
		} `json:"metricStat"`
		ReturnData bool `json:"returnData"`
	}{
		ID:         "e6ceefb7-f504-ebf4-035d-e13798e92d3f",
		ReturnData: true,
	})
	event.Detail.Configuration.Metrics[0].MetricStat.Metric.Namespace = "AWS/EC2"
	event.Detail.Configuration.Metrics[0].MetricStat.Metric.Name = "CPUUtilization"
	event.Detail.Configuration.Metrics[0].MetricStat.Metric.Dimensions = map[string]string{
		"InstanceId": "i-03f8306225046aca5",
	}
	event.Detail.Configuration.Metrics[0].MetricStat.Period = 20
	event.Detail.Configuration.Metrics[0].MetricStat.Stat = "Average"

	body, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func cloudWatchTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000-0700")
}

func sqliteTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}

type testDataPaths struct {
	dbPath     string
	schemaPath string
}

func testPaths() (testDataPaths, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return testDataPaths{}, fmt.Errorf("could not resolve script path")
	}
	scriptDir := filepath.Dir(filename)
	return testDataPaths{
		dbPath:     filepath.Clean(filepath.Join(scriptDir, "..", "data", "test-database-1.sqlite")),
		schemaPath: filepath.Clean(filepath.Join(scriptDir, "..", "..", "db-sqlc", "database.sql")),
	}, nil
}

func removeSQLiteFiles(dbPath string) error {
	if strings.HasPrefix(dbPath, "file:") {
		return fmt.Errorf("target db path should be a filesystem path, not a sqlite URI: %s", dbPath)
	}
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing sqlite file %s: %w", path, err)
		}
	}
	return nil
}

func ptr[T any](value T) *T {
	return &value
}
