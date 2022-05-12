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
	"github.com/ExpediaGroup/flyte-client/flyte"
	"github.com/ExpediaGroup/flyte-slack/types"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
)

type client interface {
	GetUserInfo(userId string) (*slack.User, error)
	NewOutgoingMessage(message, channelId string, options ...slack.RTMsgOption) *slack.OutgoingMessage
	SendMessage(message *slack.OutgoingMessage)
	PostMessage(channel string, opts ...slack.MsgOption) (string, string, error)
	GetConversations(params *slack.GetConversationsParameters) (channels []slack.Channel, nextCursor string, err error)
	ListReactions(params slack.ListReactionsParameters) ([]slack.ReactedItem, *slack.Paging, error)
}

// our slack implementation makes consistent use of channel id
type Slack interface {
	SendMessage(message, channelId, threadTimestamp string)
	SendRichMessage(rm RichMessage) (respChannel string, respTimestamp string, err error)
	IncomingMessages() <-chan flyte.Event
	// GetConversations is a heavy call used to fetch data about all channels in a workspace
	// intended to be cached, not called each time this is needed
	GetConversations() ([]types.Conversation, error)
	GetReactionMessageText(count int, user string, channelId, threadTimestamp string) (text string, err error)
}

type slackClient struct {
	client client
	// events received from slack
	incomingEvents chan slack.RTMEvent
	// messages to be consumed by API (filtered incoming events)
	incomingMessages chan flyte.Event
}

func NewSlack(token string) Slack {

	rtm := slack.New(token).NewRTM()
	go rtm.ManageConnection()

	sl := &slackClient{
		client:           rtm,
		incomingEvents:   rtm.IncomingEvents,
		incomingMessages: make(chan flyte.Event),
	}

	log.Info().Msg("initialized slack")
	go sl.handleMessageEvents()
	return sl
}

const (
	getConversationsLimit = 1000 // max 1000
	excludeArchived = true
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
	log.Info().Msgf("message=%q sent to channel=%s", msg.Text, channelId)

}

func (sl *slackClient) SendRichMessage(rm RichMessage) (string, string, error) {
	respChannel, respTimestamp, err := rm.Post(sl.client)
	if err != nil {
		return "", "", errors.New(fmt.Sprintf("cannot send rich message=%v: %v", rm, err))
	}
	log.Info().Msgf("rich message=%+v sent to channel=%s", rm, rm.ChannelID)
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
			log.Debug().Msgf("received message=%s in channel=%s", v.Text, v.Channel)
			u, err := sl.client.GetUserInfo(v.User)
			if err != nil {
				log.Err(err).Msgf("cannot get info about user=%s", v.User)
				continue
			}
			sl.incomingMessages <- toFlyteMessageEvent(v, u)
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

func (sl *slackClient) GetReactionMessageText(count int, user string, channelId, threadTimestamp string) (text string, err error) {
	params := slack.ListReactionsParameters{
		Count: count,
		User:  user,
	}
	log.Debug().Msgf("Count = %v user = %s channel id= %v  threadTimestamp= %v", count, user, channelId, threadTimestamp)

	reaction, paging, err := sl.client.ListReactions(params)
	if err != nil {
		log.Debug().Msgf("Error = %v", err)
		return "", err
	}

	for i := range reaction {
		if reaction[i].Type == "message" {
			if reaction[i].Channel == channelId {
				if reaction[i].Message.Timestamp == threadTimestamp {
					log.Debug().Msgf("reaction match found added:  Value of Type = %v , channel = %v Msg Timestamp = %v , Text = %v",
						reaction[i].Type, reaction[i].Channel, reaction[i].Message.ThreadTimestamp,
						reaction[i].Message.Text)
					return reaction[i].Message.Text, nil
				} else {
					log.Debug().Msgf("reaction match not found with provided Timestamp")
				}

			} else {
				log.Debug().Msgf("reaction match not found with provided Channel")
			}
		}
	}

	logger.Debugf("Value of paging  = %v", paging)
	return "", nil

}
