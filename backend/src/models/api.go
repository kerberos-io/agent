package models

type APIResponse struct {
	Data 	  interface{} 		`json:"data" bson:"data"`
	Count 	  interface{} 		`json:"count,omitempty" bson:"count,omitempty"`
}
