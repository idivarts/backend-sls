package main

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func main() {
	socials, err := trendlybq.SocialsN8N{}.GetPaginated(1001, 10000)
	if err != nil {
		panic(err)
	}
	log.Println("Fetching and updating", len(socials), "socials")
	for i, social := range socials {
		log.Println("updating social:", i, "/", len(socials), ":", social.ID)
		err = social.UpdateMinified()
		if err != nil {
			log.Println("error updating social:", social.ID, err.Error())
		}
	}
	log.Println("done updating socials")
}
