

Copy the database both the RDB and the firestore.

For RDB copy, use this command -

```
DROP DATABASE IF EXISTStrendly_prod;
CREATE DATABASE trendly_prod
WITH TEMPLATE trendly;
```


This is for migration of DB

- I guess no need to delete the databse. Selective copy of data can be used though using the collection group. Refer doc

```
gcloud dataflow flex-template run "firestore-job-prod-migration-4" \
--template-file-gcs-location gs://dataflow-templates-us-central1/latest/flex/Cloud_Firestore_to_Firestore \
--region us-central1 \
--parameters sourceProjectId=trendly-9ab99,sourceDatabaseId=,destinationDatabaseId=trendly-prod
```
