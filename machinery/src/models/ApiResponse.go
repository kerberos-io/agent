package models

type APIResponse struct {
	Data 	  interface{} 		`json:"data" bson:"data"`
}
