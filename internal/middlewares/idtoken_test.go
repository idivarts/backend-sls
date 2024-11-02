package middlewares_test

import (
	"context"
	"log"
	"testing"

	"github.com/TrendsHub/th-backend/pkg/firebase/fauth"
)

func TestCreateIDToken(m *testing.T) {
	uid := "0rdPB7B5q3cUvbu1Ewarp4Xg2AD3"
	data, err := fauth.Client.CustomToken(context.Background(), uid)
	if err != nil {
		m.Fail()
		return
	}
	log.Println(data)
}

func TestVerifyIdToken(t *testing.T) {
	idToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJmaXJlYmFzZS1hZG1pbnNkay0zY2FzaEBjcm93ZHktY2hhdC5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsImF1ZCI6Imh0dHBzOi8vaWRlbnRpdHl0b29sa2l0Lmdvb2dsZWFwaXMuY29tL2dvb2dsZS5pZGVudGl0eS5pZGVudGl0eXRvb2xraXQudjEuSWRlbnRpdHlUb29sa2l0IiwiZXhwIjoxNzI1Mzg0MTA4LCJpYXQiOjE3MjUzODA1MDgsInN1YiI6ImZpcmViYXNlLWFkbWluc2RrLTNjYXNoQGNyb3dkeS1jaGF0LmlhbS5nc2VydmljZWFjY291bnQuY29tIiwidWlkIjoiMHJkUEI3QjVxM2NVdmJ1MUV3YXJwNFhnMkFEMyJ9.xyxbn4I3sNccDM5OmhXmKmg2PBPK6RI80U9qn5jj5l9amK1Mrm1T2kN5fS0EIqYgNBB_4vFraqgjeVvUGd2hl9WzcpXuJEYKgMrUWJo7Km5-HEYwpyWqA-TvoExCjXJVlPf6SqElhTLJxs_AcXdU_diPqlPcURoZ_pqMva8_evjethQu0KfOHPhX_X1mVp3aJkdCD9NvOZKKvtifVl67bScghRn2XvviNFgPrwKB1WCU-4a5pPftcdVPutwv3Oc1ptd2w2jA-ZVmEqtJ23StqK6QmdLnPW6NWPhJHJ7rrFln2ifbmvh-MqAguyPhr1KfA-7GFclk1qtSd57hQkAMRg"
	token, err := fauth.Client.VerifyIDToken(context.Background(), idToken)
	if err != nil {
		log.Println(err.Error())
		t.Fail()
		return
	}
	log.Println(token)
}

// func TestValidateFunction(t *testing.T) {
// 	val := middlewares.ValidateUserOrganization("0rdPB7B5q3cUvbu1Ewarp4Xg2AD3", "jJLOC1LfG8WLgmAs5Ka7")
// 	if !val {
// 		t.Fail()
// 	}
// }
