/*
Copyright (C) 2018 Expedia Group.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"errors"
	"fmt"
	"github.com/ExpediaGroup/flyte-slack/types"
	"github.com/HotelsDotCom/flyte-client/flyte"
	"github.com/HotelsDotCom/go-logger"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"log"
	"os"
)

type client interface {
	GetUserInfo(userId string) (*slack.User, error)
	NewOutgoingMessage(message, channelId string, options ...slack.RTMsgOption) *slack.OutgoingMessage
	SendMessage(message *slack.OutgoingMessage)
	PostMessage(channel string, opts ...slack.MsgOption) (string, string, error)
	GetConversations(params *slack.GetConversationsParameters) (channels []slack.Channel, nextCursor string, err error)
	GetReactions(item slack.ItemRef, params slack.GetReactionsParameters) (reactions []slack.ItemReaction, err error)
	ListReactions(params slack.ListReactionsParameters) ([]slack.ReactedItem, *slack.Paging, error)
}

type socketclient interface {
}

type Slacksocketclient interface {
	IncomingMessages() <-chan flyte.Event
}

// our slack implementation makes consistent use of channel id
type Slack interface {
	SendMessage(message, channelId, threadTimestamp string)
	SendRichMessage(rm RichMessage) (respChannel string, respTimestamp string, err error)
	IncomingMessages() <-chan flyte.Event
	// GetConversations is a heavy call used to fetch data about all channels in a workspace
	// intended to be cached, not called each time this is needed
	GetConversations() ([]types.Conversation, error)
	GetReactions(channelId, timestamp string)
	ListReactions(count int, user string, channelId, threadTimestamp string) (text string)

	//GetReactions(channelId, threadTimestamp string)
}

type slackClient struct {
	client client
	// events received from slack
	incomingEvents chan slack.RTMEvent
	// messages to be consumed by API (filtered incoming events)
	incomingMessages chan flyte.Event
}

type slackSocketClient struct {
	client socketclient
	// events received from slack
	incomingEvents chan socketmode.Event
	// messages to be consumed by API (filtered incoming events)
	incomingMessages chan flyte.Event
}

func (apiClient *slackSocketClient) IncomingMessages() <-chan flyte.Event {
	return apiClient.incomingMessages
}

func NewSlack(token string) Slack {

	rtm := slack.New(token).NewRTM()
	go rtm.ManageConnection()

	sl := &slackClient{
		client:           rtm,
		incomingEvents:   rtm.IncomingEvents,
		incomingMessages: make(chan flyte.Event),
	}

	logger.Info("initialized slack")
	go sl.handleMessageEvents()
	return sl
}

func NewSocketBasedSlack(appToken string, botToken string) Slacksocketclient {

	slackapi := slack.New(
		botToken,
		slack.OptionDebug(true),
		slack.OptionLog(log.New(os.Stdout, "api: ", log.Lshortfile|log.LstdFlags)),
		slack.OptionAppLevelToken(appToken),
	)

	smodeclient := socketmode.New(
		slackapi,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	apiClient := &slackSocketClient{
		client:           smodeclient,
		incomingEvents:   smodeclient.Events,
		incomingMessages: make(chan flyte.Event),
	}

	go apiClient.handleSocketBasedEvents(smodeclient)

	go smodeclient.Run()

	logger.Info("initialized slack")

	return apiClient

}

func (apiClient *slackSocketClient) handleSocketBasedEvents(smodeclient *socketmode.Client) {
	logger.Debugf("Calling")
	for evt := range apiClient.incomingEvents {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			fmt.Println("Connecting to Slack with Socket Mode...")
		case socketmode.EventTypeConnectionError:
			fmt.Println("Connection failed. Retrying later...")
		case socketmode.EventTypeConnected:
			fmt.Println("Connected to Slack with Socket Mode.")

		case socketmode.EventTypeEventsAPI:

			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			logger.Debugf("EventTypeAPI recieved !!")
			fmt.Println("EventTypeAPI Recieved!!!!!")

			if !ok {

				logger.Debugf("Ignored %+v\n", evt)

				continue
			}

			logger.Debugf("Event received: %+v\n", eventsAPIEvent)

			smodeclient.Ack(*evt.Request)

			switch eventsAPIEvent.Type {

			case slackevents.Message:
				logger.Debugf("This is message evernt succc")

			case slackevents.CallbackEvent:
				innerEvent := eventsAPIEvent.InnerEvent
				switch ev := innerEvent.Data.(type) {

				case *slackevents.AppMentionEvent:
					logger.Debugf(" app mention Event recieved %v", ev)

				case *slackevents.MemberJoinedChannelEvent:
					fmt.Printf("user %q joined to channel %q", ev.User, ev.Channel)

				case *slackevents.MessageEvent:
					logger.Debugf(" Message Event recieved %v", ev)

				}
			default:
				logger.Debugf("unsupported Events API event received")
			}
		case socketmode.EventTypeInteractive:
			callback, ok := evt.Data.(slack.InteractionCallback)
			if !ok {
				fmt.Printf("Ignored %+v\n", evt)

				continue
			}

			fmt.Printf("Interaction received: %+v\n", callback)

			var payload interface{}

			switch callback.Type {
			case slack.InteractionTypeBlockActions:
				// See https://api.slack.com/apis/connections/socket-implement#button

				logger.Debugf("button clicked!")
			case slack.InteractionTypeShortcut:
			case slack.InteractionTypeViewSubmission:
				// See https://api.slack.com/apis/connections/socket-implement#modal
			case slack.InteractionTypeDialogSubmission:
			default:

			}

			smodeclient.Ack(*evt.Request, payload)
		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				fmt.Printf("Ignored %+v\n", evt)

				continue
			}

			logger.Debugf("Slash command received: %+v", cmd)

			payload := map[string]interface{}{
				"blocks": []slack.Block{
					slack.NewSectionBlock(
						&slack.TextBlockObject{
							Type: slack.MarkdownType,
							Text: "foo",
						},
						nil,
						slack.NewAccessory(
							slack.NewButtonBlockElement(
								"",
								"somevalue",
								&slack.TextBlockObject{
									Type: slack.PlainTextType,
									Text: "bar",
								},
							),
						),
					),
				}}

			smodeclient.Ack(*evt.Request, payload)

		default:
			fmt.Fprintf(os.Stderr, "Unexpected event type received: %s\n", evt.Type)
		}
	}

}

const (
	getConversationsLimit = 1000 // max 1000
	excludeArchived       = true
)

func (sl *slackClient) GetConversations() ([]types.Conversation, error) {
	params := &slack.GetConversationsParameters{
		ExcludeArchived: excludeArchived,
		Limit:           getConversationsLimit,
	}

	chans, cursor, err := sl.client.GetConversations(params)
	if err != nil {
		return nil, err
	}

	out := make([]types.Conversation, 0, len(chans))
	for i := range chans {
		out = append(out, types.Conversation{
			ID:    chans[i].ID,
			Name:  chans[i].Name,
			Topic: chans[i].Topic.Value,
		})
	}

	for cursor != "" {
		params.Cursor = cursor

		chans, cursor, err = sl.client.GetConversations(params)
		if err != nil {
			return nil, err
		}

		for i := range chans {
			out = append(out, types.Conversation{
				ID:    chans[i].ID,
				Name:  chans[i].Name,
				Topic: chans[i].Topic.Value,
			})
		}
	}

	return out, nil
}

// Sends slack message to provided channel. Channel does not have to be joined.
func (sl *slackClient) SendMessage(message, channelId, threadTimestamp string) {

	msg := sl.client.NewOutgoingMessage(message, channelId)
	msg.ThreadTimestamp = threadTimestamp
	sl.client.SendMessage(msg)
	logger.Infof("message=%q sent to channel=%s", msg.Text, channelId)

}

func (sl *slackClient) SendRichMessage(rm RichMessage) (string, string, error) {
	respChannel, respTimestamp, err := rm.Post(sl.client)
	if err != nil {
		return "", "", errors.New(fmt.Sprintf("cannot send rich message=%v: %v", rm, err))
	}
	logger.Infof("rich message=%+v sent to channel=%s", rm, rm.ChannelID)
	return respChannel, respTimestamp, nil
}

// Returns channel with incoming messages from all joined channels.
func (sl *slackClient) IncomingMessages() <-chan flyte.Event {
	return sl.incomingMessages
}

func (sl *slackClient) handleMessageEvents() {
	for event := range sl.incomingEvents {
		switch v := event.Data.(type) {
		case *slack.MessageEvent:
			logger.Debugf("received message=%s in channel=%s", v.Text, v.Channel)
			u, err := sl.client.GetUserInfo(v.User)
			if err != nil {
				logger.Errorf("cannot get info about user=%s: %v", v.User, err)
				continue
			}
			sl.incomingMessages <- toFlyteMessageEvent(v, u)

		case *slack.ReactionAddedEvent:
			logger.Debugf("received reaction event for type = %v", v)
			u, err := sl.client.GetUserInfo(v.User)
			if err != nil {
				logger.Errorf("cannot get info about user=%s: %v", v.User, err)
				continue
			}
			sl.incomingMessages <- toFlyteReactionAddedEvent(v, u)

		default:
			logger.Debugf("received  message for type = %v", v)

		}
	}
}

func toFlyteMessageEvent(event *slack.MessageEvent, user *slack.User) flyte.Event {

	return flyte.Event{
		EventDef: flyte.EventDef{Name: "ReceivedMessage"},
		Payload:  newMessageEvent(event, user),
	}
}

type messageEvent struct {
	ChannelId       string        `json:"channelId"`
	User            user          `json:"user"`
	Message         string        `json:"message"`
	Timestamp       string        `json:"timestamp"`
	ThreadTimestamp string        `json:"threadTimestamp"`
	ReplyCount      int           `json:"replyCount"`
	Replies         []slack.Reply `json:"replies"`
}

func newMessageEvent(e *slack.MessageEvent, u *slack.User) messageEvent {
	return messageEvent{
		ChannelId:       e.Channel,
		User:            newUser(u),
		Message:         e.Text,
		Timestamp:       e.Timestamp,
		ThreadTimestamp: getThreadTimestamp(e),
		ReplyCount:      e.ReplyCount,
		Replies:         e.Replies,
	}
}

func getThreadTimestamp(e *slack.MessageEvent) string {
	if e.ThreadTimestamp != "" {
		return e.ThreadTimestamp
	}
	return e.Timestamp
}

type user struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Title     string `json:"title"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func newUser(u *slack.User) user {

	return user{
		Id:        u.ID,
		Name:      u.Name,
		Email:     u.Profile.Email,
		Title:     u.Profile.Title,
		FirstName: u.Profile.FirstName,
		LastName:  u.Profile.LastName,
	}
}

func newReactionEvent(e *slack.ReactionAddedEvent, u *slack.User) reactionAddedEvent {
	return reactionAddedEvent{
		ReactionUser:   newUser(u),
		ReactionName:   e.Reaction,
		EventTimestamp: e.EventTimestamp,
		ItemTimestamp:  e.Item.Timestamp,
		ItemType:       e.Item.Type,
		ChannelId:      e.Item.Channel,
	}

}

type reactionAddedEvent struct {
	ReactionUser   user   `json:"user"`
	ReactionName   string `json:"reaction"`
	EventTimestamp string `json:"eventTimestamp"`
	ItemType       string `json:"type"`
	ItemTimestamp  string `json:"itemTimestamp"`
	ItemUser       user   `json:"itemUser"`
	ChannelId      string `json:"channelId"`
}

func toFlyteReactionAddedEvent(event *slack.ReactionAddedEvent, user *slack.User) flyte.Event {

	return flyte.Event{
		EventDef: flyte.EventDef{Name: "ReactionAdded"},
		Payload:  newReactionEvent(event, user),
	}
}

const (
	includeFull = false
)

func (sl *slackClient) GetReactions(channelId, threadTimestamp string) {

	params := slack.GetReactionsParameters{
		Full: includeFull,
	}
	logger.Debugf("Timestamp = %s Channel Id = %s", threadTimestamp, channelId)
	items := slack.ItemRef{
		channelId,
		threadTimestamp,
		"",
		"",
	}
	reaction, err := sl.client.GetReactions(items, params)
	if err != nil {
		logger.Debugf("Error = %v", err)
	}
	logger.Debugf("Value of reaction initial = %v", reaction)

	out := make([]slack.ItemReaction, 0, len(reaction))
	for i := range reaction {
		logger.Debugf("Value of reaction = %v", i)
	}

	logger.Debugf("Value of reaction out = %v", out)

}

func (sl *slackClient) ListReactions(count int, user string, channelId, threadTimestamp string) (text string) {
	logger.Debugf("Count = %v user = %s channel id= %v  threadTimestamp= %v", count, user, channelId, threadTimestamp)

	params := slack.ListReactionsParameters{
		Count: count,
		User:  user,
	}
	logger.Debugf("Count = %v user = %s channel id= %v  threadTimestamp= %v", count, user, channelId, threadTimestamp)

	reaction, paging, err := sl.client.ListReactions(params)
	if err != nil {
		logger.Debugf("Error = %v", err)
	}

	for i := range reaction {
		if reaction[i].Type == "message" {
			logger.Debugf("Chnnel = %v", reaction[i].Channel)
			if reaction[i].Channel == channelId {
				logger.Debugf("timestamp = %v", reaction[i].Message.ThreadTimestamp)
				if reaction[i].Message.Timestamp == threadTimestamp {
					logger.Debugf("Value of Type = %v , channel = %v Msg Timestamp = %v , Text = %v",
						reaction[i].Type, reaction[i].Channel, reaction[i].Message.ThreadTimestamp,
						reaction[i].Message.Text)
					return reaction[i].Message.Text
				}

			}

		}
		//logger.Debugf("Value of Type = %v , channel = %v Msg Timestamp = %v , Text = %v", reaction[i].Type,
		//reaction[i].Channel, reaction[i].Message.ThreadTimestamp, reaction[i].Message.Text)
	}

	logger.Debugf("Value of paging  = %v", paging)

	return ""

}
