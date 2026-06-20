package wshandler

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func GetUserConnections(userID string) ([]string, error) {
	return trendlymodels.WebsocketConnectionIDsByUser(userID)
}

func SendToUser(userID string, data string) {
	conns, err := GetUserConnections(userID)
	if err != nil {
		log.Printf("ws SendToUser: lookup failed for %s: %v", userID, err)
		return
	}
	for i := range conns {
		id := conns[i]
		SendToConnection(&id, data)
	}
}

func SendToConnections(conns []string, data string) {
	for i := range conns {
		id := conns[i]
		SendToConnection(&id, data)
	}
}
