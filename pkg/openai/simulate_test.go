package openai_test

import (
	"testing"
	"time"

	"github.com/TrendsHub/th-backend/pkg/openai"
)

func TestXxx(t *testing.T) {
	thread, err := openai.CreateThread()
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	openai.SendMessage(thread.ID, "Hello Rahul")
	openai.SendMessage(thread.ID, "This is Arjun")

	openai.StartRun(thread.ID, openai.ArjunAssistant)
	time.Sleep(5 * time.Second)

	msgs, err := openai.GetMessages(thread.ID)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	t.Log(msgs.Data[0].Content[0].Text.Value)
}
