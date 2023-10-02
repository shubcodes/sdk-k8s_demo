package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
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
	twilioClient   *twilio.RestClient
)

func main() {
	twilioClient = twilio.NewRestClient()

	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	tun, err := ngrok.Listen(ctx,
		config.LabeledTunnel(
			config.WithLabel("edge", "edghts_2Silclka0oSdqieuVpreBzyrDvT"),
		),
		ngrok.WithAuthtoken(os.Getenv("NGROK_AUTHTOKEN")),
	)
	if err != nil {
		return err
	}

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	http.HandleFunc("/send", handleSendMessage)
	http.HandleFunc("/receive", handleReceiveMessage)
	http.HandleFunc("/past_messages", handlePastMessages)
	http.HandleFunc("/sms", handleIncomingSMS)

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

	// Send SMS message
	//params := &api.CreateMessageParams{}
	params := &api.ListMessageParams{}
	params.SetTo("+13475579424")   // Replace with the recipient's phone number
	params.SetFrom("+18669904069") // Replace with your Twilio phone number
	params.SetBody(message.Username)
	_, err = twilioClient.Api.CreateMessage(params)
	if err != nil {
		log.Println("Error sending SMS message:", err)
	}
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

func handleIncomingSMS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.FormValue("From")
	content := r.FormValue("Body")

	log.Printf("Received SMS message from %s: %s", username, content)

	if username == "" || content == "" {
		log.Printf("Invalid SMS message: %v", r.Form)
		return
	}

	message := Message{
		ID:       nextMessageID,
		Username: username,
		Content:  content,
	}
	nextMessageID++

	broadcast <- message
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
