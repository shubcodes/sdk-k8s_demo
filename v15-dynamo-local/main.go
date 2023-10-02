package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"

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
	nextClientID   = 1                          // Next client ID
	nextMessageID  = 1                          // Next message ID
	pollWaitPeriod = 30 * time.Second           // Maximum wait period for long poll
	sess           = session.Must(session.NewSession(&aws.Config{Region: aws.String("us-west-2")}))
	dynamoDB       = dynamodb.New(sess, aws.NewConfig().WithEndpoint("http://localhost:8000"))
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	tun, err := ngrok.Listen(ctx,
		config.LabeledTunnel(
			config.WithLabel("edge", "edghts_2SiyTgOn37HKpq5RzDrGYz58YnP"),
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

	// Generate a unique ID for the message
	message.ID = generateUniqueID()

	item, err := dynamodbattribute.MarshalMap(message)
	if err != nil {
		http.Error(w, "DynamoDB MarshalMap error", http.StatusInternalServerError)
		return
	}

	input := &dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String("ChatMessages"),
	}
	if _, err := dynamoDB.PutItem(input); err != nil {
		http.Error(w, "DynamoDB PutItem error", http.StatusInternalServerError)
		return
	}

	broadcast <- message
}

// Generate a unique ID for each message
func generateUniqueID() int {
	// Implement your logic here to generate a unique ID,
	// such as using a timestamp or a UUID library.
	// Make sure the generated ID is unique for each message.
	// In this example, a simple incrementing counter is used.
	mutex.Lock()
	id := nextMessageID
	nextMessageID++
	mutex.Unlock()
	return id
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

	output, err := dynamoDB.Scan(&dynamodb.ScanInput{
		TableName: aws.String("ChatMessages"),
	})
	if err != nil {
		log.Printf("Failed to get past messages from DynamoDB: %v", err)
		http.Error(w, "Failed to get past messages", http.StatusInternalServerError)
		return
	}

	var messages []Message
	for _, item := range output.Items {
		var message Message
		err = dynamodbattribute.UnmarshalMap(item, &message)
		if err != nil {
			log.Printf("Failed to unmarshal DynamoDB item: %v", err)
			http.Error(w, "Failed to get past messages", http.StatusInternalServerError)
			return
		}
		messages = append(messages, message)
	}

	if len(messages) == 0 {
		messages = []Message{} // Ensure an empty array is returned if there are no past messages
	}

	err = json.NewEncoder(w).Encode(messages)
	if err != nil {
		log.Printf("Failed to encode messages as JSON: %v", err)
		http.Error(w, "Failed to get past messages", http.StatusInternalServerError)
		return
	}
}

func broadcastMessages() {
	for message := range broadcast {
		mutex.Lock()
		for _, client := range clients {
			client <- message
		}
		mutex.Unlock()
	}
}
