package rdb_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/rdb"
)

func TestInitMySQL(t *testing.T) {
	r, err := rdb.DB.Exec("show tables")
	if err != nil {
		t.Error(err)
	}
	log.Println(r.RowsAffected())
}
