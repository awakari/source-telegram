package pub

import (
	"context"
	"errors"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
)

type mock struct {
}

func NewMock() Service {
	return mock{}
}

func (m mock) Publish(ctx context.Context, evt *pb.CloudEvent, groupId, userId string) (err error) {
	switch userId {
	case "fail":
		err = errors.New("fail")
	case "noack":
		err = ErrNoAck
	}
	return
}
