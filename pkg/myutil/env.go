package myutil

import "os"

func IsDevEnvironment() bool {
	return os.Getenv("STAGE") == "dev"
}
