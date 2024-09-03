package middlewares_test

import (
	"context"
	"log"
	"testing"

	"github.com/TrendsHub/th-backend/pkg/firebase/auth"
)

func TestIDToken(m *testing.T) {
	uid := "0rdPB7B5q3cUvbu1Ewarp4Xg2AD3"
	data, err := auth.Client.CustomToken(context.Background(), uid)
	if err != nil {
		m.Fail()
		return
	}
	log.Println(data)
}
