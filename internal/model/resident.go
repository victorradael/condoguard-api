package model

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
)

type Resident struct {
    ID         primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
    UnitNumber string            `bson:"unitNumber" json:"unitNumber"`
    Floor      int               `bson:"floor" json:"floor"`
    Owner      *User             `bson:"owner" json:"owner"`
} 