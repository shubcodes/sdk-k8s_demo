package main

// Import necessary packages
import (
	"log"
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
	// Create a new message
	message := Message{
		ID:       1,
		Username: "TestUser",
		Content:  "Test message",
	}

	// Marshal the message into a DynamoDB attribute map
	item, err := dynamodbattribute.MarshalMap(message)
	if err != nil {
		log.Fatal(err)
	}

	// Create the PutItem input
	input := &dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String("ChatMessages"),
	}

	// Write the item to DynamoDB
	_, err = dynamoDB.PutItem(input)
	if err != nil {
		log.Fatal(err)
	}

	// Output a success message
	log.Println("Message successfully written to DynamoDB")
}
