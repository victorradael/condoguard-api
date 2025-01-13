package model

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
    "time"
)

type Notification struct {
    ID         primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
    Message    string            `bson:"message" json:"message"`
    CreatedBy  *User             `bson:"createdBy" json:"createdBy"`
    CreatedAt  time.Time         `bson:"createdAt" json:"createdAt"`
    Residents  []Resident        `bson:"residents,omitempty" json:"residents,omitempty"`
    ShopOwners []ShopOwner       `bson:"shopOwners,omitempty" json:"shopOwners,omitempty"`
} 