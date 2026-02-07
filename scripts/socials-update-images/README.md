[DONE] Storing the state 0 in firestore from extension

A mechanism to move the state-1 old but good quality data to n8n for scrapping
    First Scenario -> 10 to 50k, with decent engagement rate people
    Second Scenario -> Only looking for influencers who are clicked more often or discovered in past

A mechanism to periodically send the state-0 data to n8n for scrapping
    EC2 to be controlled by EventBridge to fetch the data from firestore and use it to n8n

A mechanism to receive state-2 data from n8n and save in firestore
    API endpoint to receive the data from n8n

A mechanism to download the image from state-2 data and save it to state-3
    Same SQS approach

Move state-3 data to bigquery and minimize and save as state-1
    Check on each image upload queue if length is sufficient insert
    Exisiting daily update still stays just in case some is left off if the length is not fulfilled