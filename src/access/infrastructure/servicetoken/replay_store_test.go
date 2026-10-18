package servicetoken

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresReplayStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewPostgresReplayStore(db)
	until := time.Now()
	q := `INSERT INTO s2s_assertion_jti \(jti, expires_at\) VALUES \(\$1, \$2\) ON CONFLICT \(jti\) DO NOTHING`

	mock.ExpectExec(q).WithArgs("a", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if fresh, err := s.Remember(context.Background(), "a", until); err != nil || !fresh {
		t.Fatalf("primer uso: fresh=%v err=%v", fresh, err)
	}
	mock.ExpectExec(q).WithArgs("a", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	if fresh, err := s.Remember(context.Background(), "a", until); err != nil || fresh {
		t.Fatalf("repetido: fresh=%v err=%v", fresh, err)
	}
	mock.ExpectExec(q).WillReturnError(errors.New("connection refused"))
	if _, err := s.Remember(context.Background(), "b", until); err == nil {
		t.Fatal("con la base caída Remember debe fallar (el caso de uso responde 503)")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
