package websocket

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Define notification types
const (
	NotificationTypeBookingRequest  = "booking_request"
	NotificationTypeBookingResponse = "booking_response"
)

// Notification represents a message sent over WebSocket
type Notification struct {
	Type         string      `json:"type"`
	Message      string      `json:"message"`
	Data         interface{} `json:"data,omitempty"`
	UserID       string      `json:"userID,omitempty"`
	RequiresAuth bool        `json:"requiresAuth,omitempty"`
}

// Client represents a connected WebSocket client
type Client struct {
	UserID        primitive.ObjectID
	Conn          *websocket.Conn
	Authenticated bool
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients                map[primitive.ObjectID]*Client
	unauthenticatedClients map[*Client]bool
	register               chan *Client
	unregister             chan *Client
	mu                     sync.RWMutex
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:                make(map[primitive.ObjectID]*Client),
		unauthenticatedClients: make(map[*Client]bool),
		register:               make(chan *Client),
		unregister:             make(chan *Client),
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if client.Authenticated && client.UserID != primitive.NilObjectID {
				h.clients[client.UserID] = client
			} else {
				h.unauthenticatedClients[client] = true
			}
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if client.Authenticated && client.UserID != primitive.NilObjectID {
				if _, ok := h.clients[client.UserID]; ok {
					delete(h.clients, client.UserID)
				}
			} else {
				delete(h.unauthenticatedClients, client)
			}
			client.Conn.Close()
			h.mu.Unlock()
		}
	}
}

// SendToUser sends a message to a specific user
func (h *Hub) SendToUser(userID primitive.ObjectID, notification Notification) error {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("user not connected")
	}

	return client.Conn.WriteJSON(notification)
}

// AuthenticateClient moves a client from unauthenticated to authenticated state
func (h *Hub) AuthenticateClient(client *Client, userID primitive.ObjectID) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove from unauthenticated clients
	if _, ok := h.unauthenticatedClients[client]; ok {
		delete(h.unauthenticatedClients, client)
	}

	// Set client as authenticated
	client.Authenticated = true
	client.UserID = userID

	// Add to authenticated clients
	h.clients[userID] = client

	return nil
}

// BroadcastToUnauthenticated sends a message to all unauthenticated clients
func (h *Hub) BroadcastToUnauthenticated(notification Notification) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.unauthenticatedClients {
		client.Conn.WriteJSON(notification)
	}
}

// NotifyBookingRequest sends a notification to service provider about new booking
func (h *Hub) NotifyBookingRequest(serviceProviderID primitive.ObjectID, bookingData interface{}) error {
	notification := Notification{
		Type:    NotificationTypeBookingRequest,
		Message: "New booking request received",
		Data:    bookingData,
	}

	return h.SendToUser(serviceProviderID, notification)
}

// NotifyBookingResponse sends a notification to user about booking status change
func (h *Hub) NotifyBookingResponse(userID primitive.ObjectID, bookingData interface{}) error {
	notification := Notification{
		Type:    NotificationTypeBookingResponse,
		Message: "Your booking status has been updated",
		Data:    bookingData,
	}

	return h.SendToUser(userID, notification)
}
