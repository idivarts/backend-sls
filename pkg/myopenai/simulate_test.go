package myopenai_test

import (
	"testing"
	"time"

	"github.com/idivarts/backend-sls/pkg/myopenai"
)

func TestXxx(t *testing.T) {
	thread, err := myopenai.CreateThread()
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	myopenai.SendMessage(thread.ID, "Hello Rahul", nil, false)
	myopenai.SendMessage(thread.ID, "This is Arjun", nil, false)

	myopenai.StartRun(thread.ID, myopenai.ArjunAssistant, "", "")
	time.Sleep(5 * time.Second)

	msgs, err := myopenai.GetMessages(thread.ID, 2, "")
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	t.Log(msgs.Data[0].Content[0].Text.Value)
}
