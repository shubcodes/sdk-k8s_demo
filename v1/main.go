package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Message represents a chat message
type Message struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Content  string `json:"content"`
}

var (
	clients           = make(map[*websocket.Conn]bool) // Connected clients
	broadcast         = make(chan Message)             // Broadcast channel
	upgrader          = websocket.Upgrader{}           // Upgrader for WebSocket connections
	mutex             sync.Mutex                       // Mutex to synchronize access to clients map
	messages          []Message                        // In-memory storage for chat messages
	lastSentMessageID int                              // Last sent message ID
)

func main() {
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/", fs)

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	// Chat history endpoint
	http.HandleFunc("/history", getChatHistory)

	go broadcastMessages()

	log.Println("Server started. Listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	// Add client connection to clients map
	mutex.Lock()
	clients[conn] = true
	mutex.Unlock()

	// Send existing chat messages to the new client
	for _, message := range messages {
		if message.ID > lastSentMessageID {
			err := conn.WriteJSON(message)
			if err != nil {
				log.Println("WebSocket write error:", err)
				conn.Close()
				mutex.Lock()
				delete(clients, conn)
				mutex.Unlock()
				return
			}
		}
	}

	for {
		var message Message

		// Read message from WebSocket connection
		err := conn.ReadJSON(&message)
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		// Generate a unique ID for the new message
		message.ID = len(messages) + 1

		// Add message to the in-memory storage
		mutex.Lock()
		messages = append(messages, message)
		mutex.Unlock()

		// Update the last sent message ID
		lastSentMessageID = message.ID

		// Broadcast the received message to all clients
		broadcast <- message
	}

	// Remove client connection from clients map
	mutex.Lock()
	delete(clients, conn)
	mutex.Unlock()
}

func broadcastMessages() {
	for {
		message := <-broadcast

		// Send message to all connected clients
		for client := range clients {
			err := client.WriteJSON(message)
			if err != nil {
				log.Println("WebSocket write error:", err)
				client.Close()
				mutex.Lock()
				delete(clients, client)
				mutex.Unlock()
			}
		}
	}
}

func getChatHistory(w http.ResponseWriter, r *http.Request) {
	// Marshal the chat messages into JSON
	data, err := json.Marshal(messages)
	if err != nil {
		log.Println("Error marshaling chat history:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
