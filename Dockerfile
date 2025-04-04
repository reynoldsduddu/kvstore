# Stage 1: Build the Go binary using an official Go image
FROM golang:1.24.0-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum first, so we can do a cached 'go mod download'
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the project files
COPY . .

# Build the Go binary
RUN go build -o kvstore-server main.go


# Stage 2: Create a smaller final image to run your server
FROM alpine:3.17

# Create a working directory in the final container
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/kvstore-server /app/kvstore-server

# (Optional) Copy config folder
COPY --from=builder /app/config /app/config

# If you have a 'frontend' folder, copy it too:
COPY --from=builder /app/frontend /app/frontend

# Expose port 8081 by default. We can override if needed.
EXPOSE 8081

# You can set a default environment variable. We'll override it at runtime
ENV SERVER_ID=0

# By default, run the kvstore-server
CMD ["/app/kvstore-server"]

