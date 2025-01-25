package main

import (
	"io"
	"log"

	"github.com/gin-gonic/gin"
)

type Message struct {
	Body    string
	Sender  string
	ID      int
	Channel string
}

type Event struct {
	// Events are pushed to this channel by the main events-gathering routine
	Message chan Message

	// New client connections
	NewClients chan chan Message

	// Closed client connections
	ClosedClients chan chan Message

	// Total client connections
	TotalClients map[chan Message]bool
}

// stores all active client connections?
type clientChan chan Message

// counter used for message ID's
var messageCounter int = 0

// Initialize event and Start procnteessing requests
func NewServer() (event *Event) {
	event = &Event{
		Message:       make(chan Message),
		NewClients:    make(chan chan Message),
		ClosedClients: make(chan chan Message),
		TotalClients:  make(map[chan Message]bool),
	}

	go event.listen()

	return
}

func main() {
	router := gin.Default()

	stream := NewServer()

	router.POST("/message", func(c *gin.Context) {
		var newMsg Message

		if err := c.BindJSON(&newMsg); err != nil {
			return
		}

		//attach message ID
		newMsg.ID = messageCounter
		messageCounter++

		stream.Message <- newMsg

		log.Printf("%s: %s", newMsg.Sender, newMsg.Body)

	})

	router.GET("/stream", HeadersMiddleware(), stream.serveHTTP(), func(c *gin.Context) {
		v, ok := c.Get("clientChan")
		if !ok {
			return
		}
		clientChan, ok := v.(clientChan)
		if !ok {
			return
		}
		c.Stream(func(w io.Writer) bool {
			//stream message to client from message channel... apparently
			if msg, ok := <-clientChan; ok {
				c.SSEvent("message", msg)
				return true
			}
			return false
		})
	})
	// Parse Static files
	router.StaticFile("/", "./index.html")

	router.RunTLS(":2300", "./keys/chatapp.pem", "./keys/chatapp.key")
}

func getMessage(c *gin.Context) {
	var newMsg Message

	if err := c.BindJSON(&newMsg); err != nil {
		return
	}

	log.Printf("%s: %s", newMsg.Sender, newMsg.Body)
}

func (stream *Event) listen() {
	for {
		select {
		//register new client
		case client := <-stream.NewClients:
			stream.TotalClients[client] = true
			log.Printf("Client added. %d clients", len(stream.TotalClients))

			//remove client who left
		case client := <-stream.ClosedClients:
			delete(stream.TotalClients, client)
			close(client)
			log.Printf("Client left, %d registered clients", len(stream.TotalClients))

			//send message to client
		case eventMsg := <-stream.Message:
			for clientMessageChan := range stream.TotalClients {
				clientMessageChan <- eventMsg
			}
		}
	}
}

// headers
func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Next()
	}
}

func (stream *Event) serveHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {

		//make client channel
		clientChan := make(clientChan)

		//add client to list when new client joins
		stream.NewClients <- clientChan

		defer func() {
			//drain client channel so it doesnt block??? server may keep sending messages to this channel
			go func() {
				for range clientChan {

				}
			}()
			stream.ClosedClients <- clientChan
		}()

		//assign the given client channel to the request context
		c.Set("clientChan", clientChan)

		c.Next()
	}
}
