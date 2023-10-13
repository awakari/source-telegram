# Producer-Telegram

Before deploying:
```shell
kubectl create secret generic producer-telegram-tokens \
  --from-literal=id=<TG_APP_ID> \
  --from-literal=hash=<TG_APP_HASH>
```

Once deployed in K8s, it requires a manual start because of interactive Telegram authentication.
Get a pod shell, run `screen` (to be able to continue running after detaching) and then run `./app`.
