package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/shomali11/commander"
	"github.com/shomali11/proper"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"go.uber.org/zap"
)

type Event struct {
	Text string
	*slack.Client
	*slackevents.AppMentionEvent
	*proper.Properties
}

type EventHandler func(context.Context, *Event)

type Command struct {
	*commander.Command
	handler EventHandler
}

func (s *EventsAPIServer) AddEventHandler(msg string, handler EventHandler) {
	c := &Command{
		Command: commander.NewCommand(msg),
		handler: handler,
	}
	s.eventHandlers = append(s.eventHandlers, c)
}

type EventsAPIServer struct {
	Token         string
	Secret        string
	client        *slack.Client
	eventHandlers []*Command
	logger        *zap.Logger
}

func NewEventsAPIServer(token, secret string, logger *zap.Logger) *EventsAPIServer {
	return &EventsAPIServer{
		Token:  token,
		Secret: secret,
		client: slack.New(token),
		logger: logger,
	}
}

func (s *EventsAPIServer) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	verifier, err := slack.NewSecretsVerifier(r.Header, s.Secret)
	if err != nil {
		s.logger.Error("slack.NewSecretsVerifier failed", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bodyReader := io.TeeReader(r.Body, &verifier)
	body, err := ioutil.ReadAll(bodyReader)
	if err != nil {
		s.logger.Error("io.ReadAll failed", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := verifier.Ensure(); err != nil {
		s.logger.Error("(slack) verifier.Ensure() failed", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventsAPIEvent, e := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
	if e != nil {
		s.logger.Error("slackevents.ParseEvent failed", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch eventsAPIEvent.Type {
	case slackevents.URLVerification:
		s.handleURLVerification(body, w)
	case slackevents.CallbackEvent:
		s.handleCallbackEvent(eventsAPIEvent.InnerEvent, w)
	}
}

func (s *EventsAPIServer) handleURLVerification(body []byte, w http.ResponseWriter) {
	r := &slackevents.ChallengeResponse{}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text")
	if _, err := w.Write([]byte(r.Challenge)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *EventsAPIServer) handleCallbackEvent(innerEvent slackevents.EventsAPIInnerEvent, w http.ResponseWriter) {
	e := &Event{Client: s.client}
	switch event := innerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		e.AppMentionEvent = event
		e.Text = event.Text
	default:
		s.logger.Error("handleCallbackEvent not implemented", zap.String("type", innerEvent.Type))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.handleEvent(e, w)
}

func (s *EventsAPIServer) handleEvent(event *Event, w http.ResponseWriter) {
	for _, cmd := range s.eventHandlers {
		props, isMatch := cmd.Command.Match(event.Text)
		if isMatch {
			ctx := context.Background()
			event.Properties = props
			cmd.handler(ctx, event)
			return
		}
	}
	// couldn't find the suitable command
	w.WriteHeader(http.StatusInternalServerError)
}
