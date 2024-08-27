package websocket

import (
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
)

var (
	firestoreClient = firestoredb.Client
	// tableName       = os.Getenv("WS_CONNECTION_TABLE")
)
