# Start from the base Go image
FROM golang:1.17-alpine as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o main .

FROM alpine 

COPY --from=builder /app/main /usr/local/bin/dynamo-k8s
# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["dynamo-k8s"]
