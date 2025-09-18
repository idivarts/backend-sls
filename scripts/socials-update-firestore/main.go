package main

import (
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func main() {
	os.Setenv("SEND_MESSAGE_QUEUE_ARN", "ScrapeImageQueue")

	executeOnAll()
}

func executeOnAll() {
	socials, err := trendlybq.Socials{}.GetPaginated(0, 700)
	if err != nil {
		log.Println("Error", err)
		return
	}
	for i, v := range socials {
		err = v.InsertToFirestore()
		if err != nil {
			log.Println("-------> Error in inserting to firebase", i, v.ID)
			continue
		}
		log.Println("Inserted", i, v.ID)
	}
	log.Println("Done All", len(socials))
}
