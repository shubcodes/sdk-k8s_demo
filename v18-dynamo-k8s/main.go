package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/gorilla/websocket"
)

type Message struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Content  string `json:"content"`
}

var (
	upgrader      = websocket.Upgrader{}
	clients       = make(map[*websocket.Conn]bool) // Connected clients
	broadcast     = make(chan Message)             // Broadcast channel
	mutex         sync.Mutex                       // Mutex to synchronize access to clients map
	nextMessageID = 1                              // Next message ID

	// AWS Session
	sess = session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"), // Adjust this to your AWS region
	}))

	// DynamoDB client
	svc = dynamodb.New(sess)
)

func main() {
	http.HandleFunc("/ws", handleConnections)
	go broadcastMessages()

	http.HandleFunc("/past_messages", handlePastMessages)
	http.Handle("/", http.FileServer(http.Dir("./static"))) // Static file server

	log.Println("Server started. Listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer ws.Close()

	clients[ws] = true

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}

		msg.ID = nextMessageID
		nextMessageID++

		av, err := dynamodbattribute.MarshalMap(msg)
		if err != nil {
			log.Fatalf("Got error marshalling new message item: %s", err)
		}

		// Explicitly add ID to the item map.
		av["ID"] = &dynamodb.AttributeValue{N: aws.String(strconv.Itoa(msg.ID))}

		_, err = svc.PutItem(&dynamodb.PutItemInput{
			TableName: aws.String("Messages"),
			Item:      av,
		})
		if err != nil {
			log.Fatalf("Got error calling PutItem: %s", err)
		}

		broadcast <- msg
	}
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
	for {
		msg := <-broadcast
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("websocket error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
