package models

type User struct {
	Installed bool   `json:"installed" bson:"installed"`
	Username  string `json:"username" bson:"username"`
	Password  string `json:"password" bson:"password"`
	Role      string `json:"role" bson:"role"`
	Language  string `json:"language" bson:"language"`
}
