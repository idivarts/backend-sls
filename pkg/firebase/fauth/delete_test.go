package fauth_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

func TestDelete(t *testing.T) {
	fauth.DeleteAllUsers()
}
