package model

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
    ID       primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
    Username string              `bson:"username" json:"username"`
    Password string              `bson:"password" json:"password,omitempty"`
    Email    string              `bson:"email" json:"email"`
    Roles    []string            `bson:"roles" json:"roles"`
    Residents []Resident         `bson:"residents,omitempty" json:"residents,omitempty"`
    ShopOwners []ShopOwner       `bson:"shopOwners,omitempty" json:"shopOwners,omitempty"`
}

type AuthRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
} 