package model

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
    "time"
)

type Expense struct {
    ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
    Description string            `bson:"description" json:"description"`
    Amount      float64           `bson:"amount" json:"amount"`
    Date        time.Time         `bson:"date" json:"date"`
    Resident    *Resident         `bson:"resident,omitempty" json:"resident,omitempty"`
    ShopOwner   *ShopOwner        `bson:"shopOwner,omitempty" json:"shopOwner,omitempty"`
} 