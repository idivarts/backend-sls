package main

import (
	"log"

	"github.com/idivarts/backend-sls/pkg/myquery"
)

func main() {
	str := myquery.Client.Project()

	log.Println("Client ProjectID", str)
}
