package myutil

import (
	"os"
)

func IsDevEnvironment() bool {
	return os.Getenv("STAGE") == "dev"
}

func IsTest() bool {
	return os.Getenv("STAGE") == ""
}
