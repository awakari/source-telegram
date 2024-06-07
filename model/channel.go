package model

import "time"

type Channel struct {
	Id      int64
	GroupId string
	UserId  string
	Name    string
	Link    string
	Created time.Time
	Last    time.Time
	SubId   string
	Terms   string
	Label   string
}
