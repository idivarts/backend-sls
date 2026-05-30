This will contain all the firestore related files like rules and tests for trendly

Before Running command, Login -

```
firebase login:add
firebase login:ci
```


Deploy code -

```
firebase deploy --only firestore:rules
firebase deploy --only firestore:indexes
```

Pull code from server -

```
firebase init firestore
```
