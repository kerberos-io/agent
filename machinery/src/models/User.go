package models

type User struct {
	Installed bool   `json:"installed" bson:"installed"`
	Username  string `json:"username" bson:"username"`
	Password  string `json:"password" bson:"password"`
	Role      string `json:"role" bson:"role"`
	Language  string `json:"language" bson:"language"`
}

type Authentication struct {
	Username string `json:"username" bson:"username"`
	Password string `json:"password" bson:"password"`
}

type Authorization struct {
	Code     int    `json:"code" bson:"code"`
	Token    string `json:"token" bson:"token"`
	Expire   string `json:"expire" bson:"expire"`
	Username string `json:"username" bson:"username"`
	Role     string `json:"role" bson:"role"`
}
