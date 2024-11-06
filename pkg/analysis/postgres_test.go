package analysis

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestPostgresDb(t *testing.T) {
	t.Run(
		"test that the postgres db struct works",
		func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("error opening a stub database connection: %v", err)
			}
			defer db.Close()

			p := &PostgresDb{
				TableName: "test",
				db:        db,
			}
			partitionName := "test_partition"
			startTime := 1234.0
			endTime := 5678.0

			rows := sqlmock.NewRows([]string{"time", "state"}).
				AddRow(1235.0, pq.Array([]float64{1.0, 2.0, 3.0})).
				AddRow(5677.0, pq.Array([]float64{4.0, 5.0, 6.0}))
			query := fmt.Sprintf(
				"SELECT time, state FROM %s WHERE partition_name = \\$1 AND time"+
					" BETWEEN \\$2 AND \\$3 ORDER BY time ASC",
				p.TableName,
			)
			mock.ExpectQuery(query).
				WithArgs(partitionName, startTime, endTime).
				WillReturnRows(rows)

			data, err := p.ReadStateInRange(partitionName, startTime, endTime)
			if err != nil {
				t.Error(err)
			}
			i := 0
			for data.Next() {
				var time float64
				var state []float64
				if err := data.Scan(&time, pq.Array(&state)); err != nil {
					t.Errorf("failed to scan row: %v", err)
				}
				if i == 0 {
					if time != 1235.0 || state[0] != 1.0 || state[1] != 2.0 ||
						state[2] != 3.0 || len(state) != 3 {
						t.Errorf(
							"values didn't match as expected: %f, %f",
							time,
							state,
						)
					}
				} else if i == 1 {
					if time != 5677.0 || state[0] != 4.0 || state[1] != 5.0 ||
						state[2] != 6.0 || len(state) != 3 {
						t.Errorf(
							"values didn't match as expected: %f, %f",
							time,
							state,
						)
					}
				} else {
					t.Error("too much data")
				}
				i += 1
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("there were unfulfilled expectations: %s", err)
			}
		})
}
