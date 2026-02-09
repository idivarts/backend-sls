package rdb_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/rdb"
)

func TestInitPostgres(t *testing.T) {
	r, err := rdb.DB.Exec("select * from socials")
	if err != nil {
		t.Error(err)
	}
	log.Println(r.RowsAffected())
}
