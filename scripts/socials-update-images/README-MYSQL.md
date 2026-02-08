API for sending the username and other manual info to SQS

CRON to see if any data needs to be rescrapped and send to SQS

Receive SQS for processing the image downloading
    -> Calling api to scrape data
        (0.0027 per call or 0.0837 per detail call)
        (0.25 per call or 7 per detail call)
    -> Download all the images
    -> Send Raw for estimations (with Bias input which were sent manually)
    -> Translate the data in Socials Data
    -> Save translated data in mysql instantly