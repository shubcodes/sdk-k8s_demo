package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

type Message struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Content  string `json:"content"`
}

var (
	clients        = make(map[int]chan Message) // Connected clients
	broadcast      = make(chan Message)         // Broadcast channel
	mutex          sync.Mutex                   // Mutex to synchronize access to clients map
	messages       []Message                    // In-memory storage for chat messages
	nextClientID   = 1                          // Next client ID
	nextMessageID  = 1                          // Next message ID
	pollWaitPeriod = 30 * time.Second           // Maximum wait period for long poll
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	tun, err := ngrok.Listen(ctx,
		config.LabeledTunnel(
			config.WithLabel("edge", "edghts_2SlOto8T8ekzyzQHBtiXIMsNlpB"),
		),
		ngrok.WithAuthtoken(os.Getenv("NGROK_AUTHTOKEN")),
	)
	if err != nil {
		return err
	}

	//log.Println("tunnel created:", tun.URL())

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	http.HandleFunc("/send", handleSendMessage)
	http.HandleFunc("/receive", handleReceiveMessage)
	http.HandleFunc("/past_messages", handlePastMessages)

	go broadcastMessages()

	return http.Serve(tun, nil)
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	decoder := json.NewDecoder(r.Body)
	var message Message
	err := decoder.Decode(&message)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	message.ID = nextMessageID
	nextMessageID++

	broadcast <- message
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}

	mutex.Lock()
	clientID := nextClientID
	clients[clientID] = make(chan Message, 1)
	nextClientID++
	mutex.Unlock()

	select {
	case message := <-clients[clientID]:
		json.NewEncoder(w).Encode(message)
	case <-time.After(pollWaitPeriod):
		w.WriteHeader(http.StatusNoContent)
	}

	mutex.Lock()
	delete(clients, clientID)
	mutex.Unlock()
}

func handlePastMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}

	json.NewEncoder(w).Encode(messages)
}

func broadcastMessages() {
	for message := range broadcast {
		messages = append(messages, message)

		mutex.Lock()
		for _, client := range clients {
			client <- message
		}
		mutex.Unlock()
	}
}
