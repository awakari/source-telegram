# Producer-Telegram

```shell
kubectl create secret generic producer-telegram-tokens \
  --from-literal=id=<TG_APP_ID> \
  --from-literal=hash=<TG_APP_HASH>
```
