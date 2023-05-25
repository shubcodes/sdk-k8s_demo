package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

const storageFile = "chat_messages.json"

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

		// Save the chat messages to file
		saveChatMessagesToFile()
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func saveChatMessagesToFile() {
	data, err := json.Marshal(messages)
	if err != nil {
		log.Println("Error marshaling chat messages:", err)
		return
	}

	err = ioutil.WriteFile(storageFile, data, 0644)
	if err != nil {
		log.Println("Error writing chat messages to file:", err)
	}
}

func loadChatMessagesFromFile() {
	data, err := ioutil.ReadFile(storageFile)
	if err != nil {
		log.Println("Error reading chat messages from file:", err)
		return
	}

	if len(data) == 0 {
		// If the file is empty, initialize an empty messages slice
		messages = []Message{}
		return
	}

	err = json.Unmarshal(data, &messages)
	if err != nil {
		log.Println("Error unmarshaling chat messages:", err)
	}
}

func init() {
	// Check if the chat_messages.json file exists, create it if it doesn't
	_, err := os.Stat(storageFile)
	if os.IsNotExist(err) {
		err := ioutil.WriteFile(storageFile, []byte("[]"), 0644)
		if err != nil {
			log.Println("Error creating chat messages file:", err)
			return
		}
	}

	loadChatMessagesFromFile()
}
