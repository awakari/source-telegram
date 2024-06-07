# source-telegram

Before deploying:
```shell
kubectl create secret generic source-telegram-tokens \
  --from-literal=ids=<TG_APP_ID_0>,<TG_APP_ID_1>,... \
  --from-literal=hashes=<TG_APP_HASH_0>,<TG_APP_HASH_1>,... \
  --from-literal=phones=<TG_PHONE_NUM_0>,<TG_PHONE_NUM_1>,...
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
