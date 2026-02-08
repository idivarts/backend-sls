Storing the state 0 in mysql 
    -> Wait for 100 such Data
    -> Send SQS for Scrapping

Receive SQS for scrapping or Cron
    -> Send SQS for Image downloading and calculation and openAI calculation

Receive SQS for image downloading 
    -> Download all the images
    -> Send Raw for estimations (with Bias input which were sent manually)
    -> Translate the data in Socials Data
    -> Save translated data in mysql instantly





A mechanism to move the state-1 old but good quality data to n8n for scrapping
    First Scenario -> 10 to 50k, with decent engagement rate people
    Second Scenario -> Only looking for influencers who are clicked more often or discovered in past

<!-- A mechanism to periodically send the state-0 data to n8n for scrapping
    EC2 to be controlled by EventBridge to fetch the data from firestore and use it to n8n -->
A mechanism to periodically scrape state-0 data usign apify
    Schedule using CRON and SQS
    Save All data as state-2
    Send them for scrapping images using SQS

<!-- A mechanism to receive state-2 data from n8n and save in firestore
    API endpoint to receive the data from n8n -->

A mechanism to download the image from state-2 data and save it to state-3
    Same SQS approach
    If length is around 100, trigger SQS to export to bigquery

Move state-3 data to bigquery and minimize and save as state-1
    Existing CRON Stays
    Listen to SQS event too