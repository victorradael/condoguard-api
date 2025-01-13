package model

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
)

type ShopOwner struct {
    ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
    ShopName string            `bson:"shopName" json:"shopName"`
    Floor    int               `bson:"floor" json:"floor"`
    Owner    *User             `bson:"owner" json:"owner"`
} 