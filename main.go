package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

func Ping(ctx context.Context, event *Event) {
	e := event.AppMentionEvent
	event.Client.PostMessage(e.Channel, slack.MsgOptionText("pong", false))
}

func main() {
	token := os.Getenv("SLACK_TOKEN")
	secret := os.Getenv("SLACK_SIGNING_SECRET")

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	es := NewEventsAPIServer(token, secret, logger)
	es.AddEventHandler("ping", Ping)

	mux := http.NewServeMux()
	mux.HandleFunc("/slack/events", es.HTTPHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:     fmt.Sprintf(":%s", port),
		Handler:  mux,
		ErrorLog: zap.NewStdLog(logger),
	}
	logger.Info("Server listening", zap.String("port", port))
	server.ListenAndServe()
}
