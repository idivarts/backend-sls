package websocket

import (
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

var (
	firestoreClient = firestoredb.Client
	// tableName       = os.Getenv("WS_CONNECTION_TABLE")
)
