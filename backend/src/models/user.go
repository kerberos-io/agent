package models

type User struct {
	Username 	string 		`json:"username" bson:"username"`
	Password 	string 		`json:"password" bson:"password"`
	Role 			string 		`json:"role" bson:"role"`
}
