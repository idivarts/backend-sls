This will contain all the firestore related files like rules and tests for trendly

Before Running command, Login -

```
firebase login --reauth
firebase login:add
firebase login:ci
```

Dev Deploy code -

```
firebase deploy --only firestore:rules
firebase deploy --only firestore:indexes
```


Prod Deploy code -

```
firebase deploy --config firebase.prod.json --only firestore:rules
firebase deploy --config firebase.prod.json --only firestore:indexes
```


Pull code from server -

```
firebase init firestore
```
