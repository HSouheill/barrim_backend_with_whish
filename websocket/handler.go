package websocket

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HandleWebSocket handles the WebSocket connection
func HandleWebSocket(c echo.Context, hub *Hub, userID primitive.ObjectID) error {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	// Create client with potentially nil userID (will be set after authentication)
	client := &Client{
		UserID:        userID,
		Conn:          conn,
		Authenticated: userID != primitive.NilObjectID,
	}

	hub.register <- client

	// Send a welcome message
	if client.Authenticated {
		conn.WriteJSON(Notification{
			Type:    "connected",
			Message: "WebSocket connection established",
			UserID:  userID.Hex(),
		})
	} else {
		conn.WriteJSON(Notification{
			Type:         "connected",
			Message:      "WebSocket connection established. Please authenticate to receive notifications.",
			RequiresAuth: true,
		})
	}

	// Handle disconnection
	go func() {
		defer func() {
			hub.unregister <- client
		}()

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// Handle authentication message
			if messageType == websocket.TextMessage {
				messageStr := string(message)
				if strings.HasPrefix(messageStr, "AUTH:") {
					// Extract token from message (format: "AUTH:token_here")
					// Here you would validate the token and set the userID
					// For now, we'll just acknowledge the auth attempt
					conn.WriteJSON(Notification{
						Type:         "auth_response",
						Message:      "Authentication received. Token validation would happen here.",
						RequiresAuth: false,
					})
					client.Authenticated = true
					continue
				}
			}
		}
	}()

	return nil
}
