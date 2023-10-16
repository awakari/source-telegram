# source-telegram

Before deploying:
```shell
kubectl create secret generic source-telegram-tokens \
  --from-literal=id=<TG_APP_ID> \
  --from-literal=hash=<TG_APP_HASH> \
  --from-literal=phone=<TG_PHONE_NUM>
```

Once deployed in K8s, it requires a manual start because of interactive Telegram authentication.
Get a pod shell, run `screen` (to be able to continue running after detaching) and then run `./app`.
