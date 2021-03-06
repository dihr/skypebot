package bot

import (
	"bytes"
	"github.com/JILeXanDR/skypebot/bot/message"
	"github.com/JILeXanDR/skypebot/skypeapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
)

const (
	privateConversation = ConversationID("8:jilexandr")
	groupConversation   = ConversationID("19:58b03afc025e48d3a34e12d370412971@thread.skype")
)

type skypeFakeServer struct {
	*Bot
	handler http.HandlerFunc
}

func newBotWrapper() *skypeFakeServer {
	b := New(Config{})
	return &skypeFakeServer{
		b,
		b.WebHookHandler(),
	}
}

func (s *skypeFakeServer) SendWebHook(jsonBody []byte) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(jsonBody))
	if err != nil {
		panic(err.Error())
		//t.Fatal(err)
	}

	s.handler.ServeHTTP(rr, req)

	return rr
}

func loadTestFile(t *testing.T, name string) []byte {
	f, err := os.Open(path.Join("./testdata", name))
	if err != nil {
		t.Fatal(err.Error())
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err.Error())
	}

	return data
}

func TestBot_WebHookHandler(t *testing.T) {
	privateMessageActivity := loadTestFile(t, "private_message_activity.json")
	groupMessageActivity := loadTestFile(t, "group_message_activity.json")
	attachmentActivity := loadTestFile(t, "attachment_message_activity.json")
	cmdMessageActivity := loadTestFile(t, "cmd_message_activity.json")

	customCmdMessageActivity := func(cmd string) []byte {
		return bytes.ReplaceAll(cmdMessageActivity, []byte("TEXT_TEMPLATE"), []byte(cmd))
	}

	t.Run("nil body", func(t *testing.T) {
		rr := newBotWrapper().SendWebHook(nil)

		require.EqualValues(t, http.StatusBadRequest, rr.Code)
		require.EqualValues(t, `bad activity: EOF`, rr.Body.String())
	})

	t.Run("empty json without massage handler", func(t *testing.T) {
		rr := newBotWrapper().SendWebHook([]byte(`{}`))

		require.EqualValues(t, http.StatusNoContent, rr.Code)
	})

	t.Run("valid activity without massage handler", func(t *testing.T) {
		rr := newBotWrapper().SendWebHook(privateMessageActivity)

		require.EqualValues(t, http.StatusNoContent, rr.Code)
	})

	t.Run("valid activity with massage handler (private message)", func(t *testing.T) {
		b := newBotWrapper()

		b.Handle(OnTextMessage, func(activity *Activity) {
			assert.False(t, activity.IsGroup())
			assert.Equal(t, "Alexandr Shtovba", activity.Sender().account.Name)
			assert.Equal(t, "test", activity.Text())
		})

		rr := b.SendWebHook(privateMessageActivity)

		require.EqualValues(t, http.StatusNoContent, rr.Code)
	})

	t.Run("valid activity with massage handler (group message)", func(t *testing.T) {
		b := newBotWrapper()
		b.Handle(OnTextMessage, func(activity *Activity) {
			assert.True(t, activity.IsGroup())
			assert.Equal(t, "Alexandr Shtovba", activity.Sender().account.Name)
			assert.Equal(t, "help", activity.Text())
		})

		rr := b.SendWebHook(groupMessageActivity)

		require.EqualValues(t, http.StatusNoContent, rr.Code)
	})

	t.Run("attachment activity", func(t *testing.T) {
		b := newBotWrapper()
		b.Handle(OnAttachment, func(activity *Activity) {
			assert.True(t, true)
		})
		b.SendWebHook(attachmentActivity)
	})

	t.Run("command activity 1", func(t *testing.T) {
		b := newBotWrapper()
		cmd := NewCommand("test", []string{"value"})
		b.Handle(cmd, func(activity *Activity) {
			cmd.Parse(activity.Text())
			assert.Equal(t, "command:test", cmd.ID())
			assert.Equal(t, "test", cmd.Name())
			assert.Equal(t, map[string]interface{}{"value": 999}, cmd.Args())
		})

		b.SendWebHook(customCmdMessageActivity("test 999"))
	})

	t.Run("command activity 2", func(t *testing.T) {
		b := newBotWrapper()
		cmd := NewCommand("send_message", []string{"text"})
		b.Handle(cmd, func(activity *Activity) {
			cmd.Parse(activity.Text())
			assert.Equal(t, "command:send_message", cmd.ID())
			assert.Equal(t, "send_message", cmd.Name())
			assert.Equal(t, map[string]interface{}{"text": "sometext"}, cmd.Args())
		})
		b.SendWebHook(customCmdMessageActivity("send_message sometext"))
	})
}

func TestBot_Send(t *testing.T) {
	b := New(Config{
		AppID:     os.Getenv("SKYPE_APP_ID"),
		AppSecret: os.Getenv("SKYPE_APP_SECRET"),
		Logger:    log.New(os.Stdout, "", 0),
	})

	require.NoError(t, b.Run())

	t.Run("to specific contact id", func(t *testing.T) {
		err := b.Send(privateConversation, message.TextMessage("test 1"))
		require.NoError(t, err)
	})

	t.Run("to specific group id", func(t *testing.T) {
		err := b.Send(groupConversation, message.TextMessage("test 2"))
		require.NoError(t, err)
	})

	t.Run("send bad image", func(t *testing.T) {
		_, err := message.NewBuilder().WithAttachmentFromURL("not url").Build()
		require.EqualError(t, err, `can't download image via URL not url: Get not%20url: unsupported protocol scheme ""`)
	})

	t.Run("send image", func(t *testing.T) {
		msg, err := message.NewBuilder().WithAttachmentFromURL("https://hackernoon.com/hn-images/0*xMaFF2hSXpf_kIfG.jpg").Build()
		require.NoError(t, err)

		err = b.Send(privateConversation, msg)
		require.NoError(t, err)
	})

	activity := &Activity{
		&skypeapi.Activity{
			From: skypeapi.ChannelAccount{
				ID: privateConversation.RecipientID(),
			},
			Conversation: skypeapi.ConversationAccount{
				ID: groupConversation.RecipientID(),
			},
		},
	}
	t.Run("reply to activity (directly to group)", func(t *testing.T) {
		err := b.Send(activity, message.TextMessage("@Alexandr Shtovba test 3"))
		require.NoError(t, err)
	})
}

func TestBot_SendActions(t *testing.T) {
	b := New(Config{
		AppID:     os.Getenv("SKYPE_APP_ID"),
		AppSecret: os.Getenv("SKYPE_APP_SECRET"),
		Logger:    log.New(os.Stdout, "", 0),
	})

	require.NoError(t, b.Run())

	actions := []skypeapi.CardAction{
		{
			Type:  "imBack",
			Title: "title1",
			Value: "value1",
		},
		{
			Type:  "imBack",
			Title: "title2",
			Value: "value2",
		},
	}

	t.Run("private message", func(t *testing.T) {
		err := b.SendActions(privateConversation, "test1", actions)
		require.NoError(t, err)
	})

	t.Run("group message", func(t *testing.T) {
		err := b.SendActions(groupConversation, "test2", actions)
		require.NoError(t, err)
	})
}
