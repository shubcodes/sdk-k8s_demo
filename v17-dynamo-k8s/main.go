package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type Message struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Content  string `json:"content"`
}

var (
	clients       = make(map[int]chan Message) // Connected clients
	broadcast     = make(chan Message)         // Broadcast channel
	mutex         sync.Mutex                   // Mutex to synchronize access to clients map
	nextClientID  = 1                          // Next client ID
	nextMessageID = 1                          // Next message ID

	// AWS Session
	sess = session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"), // Adjust this to your AWS region
	}))

	// DynamoDB client
	svc = dynamodb.New(sess)
)

func main() {
	http.HandleFunc("/send", handleSendMessage)
	http.HandleFunc("/receive", handleReceiveMessage)
	http.HandleFunc("/past_messages", handlePastMessages)
	http.Handle("/", http.FileServer(http.Dir("./static"))) // Static file server

	go broadcastMessages()

	log.Println("Server started. Listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
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

	av, err := dynamodbattribute.MarshalMap(message)
	if err != nil {
		log.Fatalf("Got error marshalling new message item: %s", err)
	}

	// Explicitly add ID to the item map.
	av["ID"] = &dynamodb.AttributeValue{N: aws.String(strconv.Itoa(message.ID))}

	_, err = svc.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("Messages"),
		Item:      av,
	})
	if err != nil {
		log.Fatalf("Got error calling PutItem: %s", err)
	}

	broadcast <- message
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}

	mutex.Lock()
	clientID := nextClientID
	clients[clientID] = make(chan Message, 100)
	nextClientID++
	mutex.Unlock()

	select {
	case message := <-clients[clientID]:
		json.NewEncoder(w).Encode(message)
	case <-time.After(30 * time.Second):
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

	result, err := svc.Scan(&dynamodb.ScanInput{
		TableName: aws.String("Messages"),
	})
	if err != nil {
		log.Fatalf("Got error calling Scan: %s", err)
	}

	messages := []Message{}

	for _, i := range result.Items {
		message := Message{}

		err = dynamodbattribute.UnmarshalMap(i, &message)
		if err != nil {
			log.Fatalf("Failed to unmarshal Record: %s", err)
		}

		messages = append(messages, message)
	}

	json.NewEncoder(w).Encode(messages)
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
