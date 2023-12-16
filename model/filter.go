package model

type ChannelFilter struct {

	// IdDiv is used together with IdRem when IdDiv is not 0 to filter the channel ids.
	IdDiv uint32

	// IdRem is used to select only channel ids that have the remainder equal to -IdRem.
	IdRem uint32

	GroupId string
	UserId  string
	Pattern string
}
