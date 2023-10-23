# source-telegram

Before deploying:
```shell
kubectl create secret generic source-telegram-tokens \
  --from-literal=id=<TG_APP_ID> \
  --from-literal=hash=<TG_APP_HASH> \
  --from-literal=phone=<TG_PHONE_NUM>
```

Once deployed in K8s, it requires a manual code input to complete the Telegram authentication.
Get a pod shell, run:
```shell
echo ##### > tgcodein
```

Example request:
```shell
grpcurl \
  -plaintext \
  -proto api/grpc/service.proto \
  -d '{ "limit": 10, "cursor": "https://t.me/astroalert"}' \
  localhost:50051 \
  awakari.source.telegram.Service/List
```
