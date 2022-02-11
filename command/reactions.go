package command

import (
	"encoding/json"
	"fmt"
	"github.com/ExpediaGroup/flyte-slack/client"
	"github.com/HotelsDotCom/flyte-client/flyte"
	"github.com/HotelsDotCom/go-logger"
	"strings"
)

var (
	getReactionEventDef       = flyte.EventDef{Name: "GetReactionSuccess"}
	getReactionFailedEventDef = flyte.EventDef{Name: "GetReactionFailed"}
)

type GetReactionInput struct {
	Message         string `json:"message"`
	ThreadTimestamp string `json:"threadTimestamp"`
	ChannelId       string `json:"channelId"`
}

type GetReactionOutput struct {
	GetReactionInput
}

type GetReactionErrorOutput struct {
	GetReactionOutput
	Error string `json:"error"`
}

func GetReactionMsg(slack client.Slack) flyte.Command {

	return flyte.Command{
		Name:         "GetReactionMsg",
		OutputEvents: []flyte.EventDef{getReactionEventDef, getReactionFailedEventDef},
		Handler:      getReactionMessageHandler(slack),
	}
}

func getReactionMessageHandler(slack client.Slack) func(json.RawMessage) flyte.Event {

	return func(rawInput json.RawMessage) flyte.Event {

		input := GetReactionInput{}
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return flyte.NewFatalEvent(fmt.Sprintf("input is not valid: %v", err))
		}

		errorMessages := []string{}
		if input.ThreadTimestamp == "" {
			errorMessages = append(errorMessages, "missing Message Timestamp field")
		}
		if input.ChannelId == "" {
			errorMessages = append(errorMessages, "missing channel id field")
		}
		if len(errorMessages) != 0 {
			return newSendMessageFailedEvent(input.Message, input.ChannelId, strings.Join(errorMessages, ", "))
		}

		slack.GetReactions(input.ChannelId, input.ThreadTimestamp)
		return newMessageSentEvent(input.Message, input.ChannelId)
	}
}

var (
	getReactionListEventDef       = flyte.EventDef{Name: "GetReactionListSuccess"}
	getReactionListFailedEventDef = flyte.EventDef{Name: "GetReactionListFailed"}
)

type GetReactionListInput struct {
	Count           int    `json:"count"`
	Message         string `json:"message"`
	ThreadTimestamp string `json:"threadTimestamp"`
	User            string `json:"reactionUser"`
	ChannelId       string `json:"channelId"`
	ItemUser        string `json:"itemUser"`
}

type GetReactionListOutput struct {
	GetReactionListInput
}

type GetReactionListErrorOutput struct {
	GetReactionListOutput
	Error string `json:"error"`
}

func GetReactionList(slack client.Slack) flyte.Command {

	return flyte.Command{
		Name:         "GetReactionList",
		OutputEvents: []flyte.EventDef{getReactionListEventDef, getReactionListFailedEventDef},
		Handler:      getReactionListHandler(slack),
	}
}

func getReactionListHandler(slack client.Slack) func(json.RawMessage) flyte.Event {

	return func(rawInput json.RawMessage) flyte.Event {
		logger.Debugf("Func called getReactionList")

		input := GetReactionListInput{}
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return flyte.NewFatalEvent(fmt.Sprintf("input is not valid: %v", err))
		}

		errorMessages := []string{}
		if input.ThreadTimestamp == "" {
			errorMessages = append(errorMessages, "missing Message Timestamp field")
		}
		if input.ChannelId == "" {
			errorMessages = append(errorMessages, "missing channel id field")
		}
		if input.User == "" {
			errorMessages = append(errorMessages, "missing user id field")
		}
		if len(errorMessages) != 0 {
			return newreactionListFailedEvent(input.Message, input.ChannelId, strings.Join(errorMessages, ", "))
		}

		issueSummary := slack.ListReactions(50, input.User, input.ChannelId, input.ThreadTimestamp)
		return newreactionListEvent(issueSummary, input.ChannelId)
	}
}

func newreactionListEvent(message, channelId string) flyte.Event {

	return flyte.Event{
		EventDef: getReactionListEventDef,
		Payload:  GetReactionListOutput{GetReactionListInput: GetReactionListInput{Message: message, ChannelId: channelId}},
	}
}

func newreactionListFailedEvent(message, channelId string, err string) flyte.Event {

	output := GetReactionListOutput{GetReactionListInput: GetReactionListInput{Message: message, ChannelId: channelId}}
	return flyte.Event{
		EventDef: getReactionFailedEventDef,
		Payload:  GetReactionListErrorOutput{GetReactionListOutput: output, Error: err},
	}
}
