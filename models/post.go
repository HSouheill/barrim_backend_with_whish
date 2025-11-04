package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Post model for media posts
type Post struct {
	ID          primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	UserID      primitive.ObjectID   `json:"userId" bson:"userId"`
	Content     string               `json:"content,omitempty" bson:"content,omitempty"`
	MediaType   string               `json:"mediaType" bson:"mediaType"` // "image" or "video"
	MediaURL    string               `json:"mediaUrl" bson:"mediaUrl"`
	ThumbnailURL string              `json:"thumbnailUrl,omitempty" bson:"thumbnailUrl,omitempty"`
	Likes       []primitive.ObjectID `json:"likes,omitempty" bson:"likes,omitempty"`
	Comments    []Comment            `json:"comments,omitempty" bson:"comments,omitempty"`
	CreatedAt   time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt" bson:"updatedAt"`
}

// Comment model for post comments
type Comment struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	UserID    primitive.ObjectID `json:"userId" bson:"userId"`
	Content   string             `json:"content" bson:"content"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// PostRequest model for creating a new post
type PostRequest struct {
	Content   string `json:"content,omitempty"`
	MediaType string `json:"mediaType"` // "image" or "video"
}

// PostResponse model for post responses
type PostResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    *Post  `json:"data,omitempty"`
}

// PostsResponse model for multiple post responses
type PostsResponse struct {
	Status  int     `json:"status"`
	Message string  `json:"message"`
	Data    []Post  `json:"data,omitempty"`
} 