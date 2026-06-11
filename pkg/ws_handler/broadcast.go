package wshandler

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func Broadcast(data string) error {
	ids, err := trendlymodels.AllWebsocketConnectionIDs()
	if err != nil {
		log.Printf("Broadcast: failed to list connections: %v", err)
		return err
	}
	for i := range ids {
		id := ids[i]
		SendToConnection(&id, data)
	}
	return nil
}
