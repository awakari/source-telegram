package service

import (
    "context"
    "fmt"
    "github.com/awakari/source-telegram/model"
    "log/slog"
)

type serviceLogging struct {
    svc Service
    log *slog.Logger
}

func NewServiceLogging(svc Service, log *slog.Logger) Service {
    return serviceLogging{
        svc: svc,
        log: log,
    }
}

func (sl serviceLogging) Create(ctx context.Context, ch model.Channel) (err error) {
    err = sl.svc.Create(ctx, ch)
    switch err {
    case nil:
        sl.log.Debug(fmt.Sprintf("service.Create(%+v): ok", ch))
    default:
        sl.log.Error(fmt.Sprintf("service.Create(%+v): %s", ch, err))
    }
    return
}

func (sl serviceLogging) Read(ctx context.Context, link string) (ch model.Channel, err error) {
    ch, err = sl.svc.Read(ctx, link)
    switch err {
    case nil:
        sl.log.Debug(fmt.Sprintf("service.Read(%s): %+v", link, ch))
    default:
        sl.log.Error(fmt.Sprintf("service.Read(%s): %s", link, err))
    }
    return
}

func (sl serviceLogging) Delete(ctx context.Context, link string) (err error) {
    err = sl.svc.Delete(ctx, link)
    switch err {
    case nil:
        sl.log.Debug(fmt.Sprintf("service.Delete(%s): ok", link))
    default:
        sl.log.Error(fmt.Sprintf("service.Delete(%s): %s", link, err))
    }
    return
}

func (sl serviceLogging) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
    page, err = sl.svc.GetPage(ctx, filter, limit, cursor, order)
    switch err {
    case nil:
        sl.log.Debug(fmt.Sprintf("service.List(%+v, %d, %s, %s): %d", filter, limit, cursor, order, len(page)))
    default:
        sl.log.Error(fmt.Sprintf("service.List(%+v, %d, %s, %s): %s", filter, limit, cursor, order, err))
    }
    return
}

func (sl serviceLogging) SearchAndAdd(ctx context.Context, groupId, subId, terms string, limit uint32) (n uint32, err error) {
    n, err = sl.svc.SearchAndAdd(ctx, groupId, subId, terms, limit)
    switch err {
    case nil:
        sl.log.Debug(fmt.Sprintf("service.SearchAndAdd(%s, %s, %s, %d): %d", groupId, subId, terms, limit, n))
    default:
        sl.log.Warn(fmt.Sprintf("service.SearchAndAdd(%s, %s, %s, %d): %d, %s", groupId, subId, terms, limit, n, err))
    }
    return
}

func (sl serviceLogging) RefreshJoinedLoop() (err error) {
    return sl.svc.RefreshJoinedLoop()
}
