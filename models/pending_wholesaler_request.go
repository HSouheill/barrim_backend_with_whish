package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PendingWholesalerRequest struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Wholesaler       Wholesaler         `bson:"wholesaler" json:"wholesaler"`
	Email            string             `bson:"email,omitempty" json:"email,omitempty"`
	AdditionalEmails []string           `bson:"additionalEmails,omitempty" json:"additionalEmails,omitempty"`
	Password         string             `bson:"password,omitempty" json:"password,omitempty"`
	SalesPersonID    primitive.ObjectID `bson:"salesPersonId" json:"salesPersonId"`
	SalesManagerID   primitive.ObjectID `bson:"salesManagerId" json:"salesManagerId"`
	Reason           string             `bson:"reason,omitempty" json:"reason,omitempty"`
	CreatedAt        time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt" json:"updatedAt"`
}
